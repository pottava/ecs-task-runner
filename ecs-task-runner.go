package etr

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	cw "github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/docker/distribution/reference"
	"github.com/google/uuid"
	"github.com/pottava/ecs-task-runner/lib"
)

// Config is set of configurations
type Config struct {
	AwsAccessKey   *string
	AwsSecretKey   *string
	AwsRegion      *string
	EcsCluster     *string
	Image          string
	ForceECR       *bool
	TaskDefFamily  *string
	Entrypoint     []*string
	Commands       []*string
	Environments   map[string]*string
	Labels         map[string]*string
	Subnets        []*string
	SecurityGroups []*string
	CPU            *string
	Memory         *string
	TaskRoleArn    *string
	NumberOfTasks  *int64
	TaskTimeout    *int64
	IsDebugMode    bool
}

var (
	resourceID string
	taskARN    *string
)

// Run runs the docker image on Amazon ECS
func Run(ctx context.Context, conf *Config) (exitCode *int64, err error) {
	resourceID = fmt.Sprintf("ecs-task-runner-%s", uuid.New().String())

	if conf.IsDebugMode {
		lib.PrintJSON(conf)
	}
	// Check AWS credentials
	sess, err := lib.Session(conf.AwsAccessKey, conf.AwsSecretKey, conf.AwsRegion, nil)
	if err != nil {
		return nil, err
	}
	account, err := sts.New(sess).GetCallerIdentityWithContext(ctx, nil)
	if err != nil {
		return nil, errors.New("Provided AWS credentials are invalid")
	}
	if conf.IsDebugMode {
		lib.PrintJSON(account)
	}
	// Check existence of the image on ECR
	image, err := validateImageName(ctx, conf, sess, aws.StringValue(account.Account))
	if err != nil {
		return nil, err
	}
	// Ensure resource existence
	if err = ensureAWSResources(ctx, sess, conf, resourceID); err != nil {
		return nil, err
	}
	// Create AWS resources
	taskARN, _, err = createResouces(ctx, sess, conf, resourceID, image)
	if err != nil {
		DeleteResouces(conf)
		return nil, err
	}
	// Run the ECS task
	tasks, err := run(ctx, sess, conf, taskARN, resourceID)
	if err != nil {
		DeleteResouces(conf)
		return nil, err
	}
	// Wait for its done
	exitCode, err = waitForTaskDone(ctx, sess, conf, tasks)
	if err != nil {
		DeleteResouces(conf)
		return nil, err
	}
	// Retrieve app log
	logs := retrieveLogs(ctx, sess, resourceID, tasks)

	// Delete AWS resources
	DeleteResouces(conf)

	// Format the result
	result := map[string][]string{}
	seq := 1
	for _, value := range logs {
		messages := []string{}
		for _, event := range value {
			messages = append(messages, fmt.Sprintf(
				"%s: %s",
				time.Unix(aws.Int64Value(event.Timestamp)/1000, 0).Format(time.RFC3339),
				aws.StringValue(event.Message),
			))
		}
		result[fmt.Sprintf("container-%d", seq)] = messages
		seq++
	}
	lib.PrintJSON(result)
	return exitCode, nil
}

func validateImageName(ctx context.Context, conf *Config, sess *session.Session, account string) (*string, error) {
	imageHost, imageName, imageTag, err := parseImageName(conf.Image)
	if err != nil {
		lib.Errors.Println("Provided image name is invalid.")
		return nil, err
	}
	// Try to make up ECR image name
	if aws.BoolValue(conf.ForceECR) && !strings.Contains(aws.StringValue(imageHost), account) {
		imageName = aws.String(fmt.Sprintf(
			"%s/%s",
			aws.StringValue(imageHost),
			aws.StringValue(imageName),
		))
		imageHost = aws.String(fmt.Sprintf(
			"%s.dkr.ecr.%s.amazonaws.com",
			account,
			aws.StringValue(conf.AwsRegion),
		))
	}
	if strings.Contains(aws.StringValue(imageHost), "amazonaws.com") {
		if _, err := ecr.New(sess).DescribeRepositoriesWithContext(ctx, &ecr.DescribeRepositoriesInput{
			RepositoryNames: []*string{imageName},
		}); err != nil {
			return nil, err
		}
	}
	if aws.StringValue(imageHost) == "" {
		return aws.String(fmt.Sprintf(
			"%s:%s",
			aws.StringValue(imageName),
			aws.StringValue(imageTag),
		)), nil
	}
	return aws.String(fmt.Sprintf(
		"%s/%s:%s",
		aws.StringValue(imageHost),
		aws.StringValue(imageName),
		aws.StringValue(imageTag),
	)), nil
}

func parseImageName(value string) (*string, *string, *string, error) {
	ref, err := reference.Parse(value)
	if err != nil {
		return nil, nil, nil, err
	}
	imageHost := ""
	imageName := ""
	if candidate, ok := ref.(reference.Named); ok {
		imageHost, imageName = reference.SplitHostname(candidate)
	}
	imageTag := "latest"
	if candidate, ok := ref.(reference.Tagged); ok {
		imageTag = candidate.Tag()
	}
	return aws.String(imageHost), aws.String(imageName), aws.String(imageTag), nil
}

func ensureAWSResources(ctx context.Context, sess *session.Session, conf *Config, id string) error {

	// Ensure cluster existence
	if conf.EcsCluster == nil || aws.StringValue(conf.EcsCluster) == "" {
		conf.EcsCluster = aws.String(id)
	}
	if err := createClusterIfNotExist(ctx, sess, conf.EcsCluster); err != nil {
		return nil
	}
	vpc := findDefaultVPC(ctx, sess)

	// Ensure subnets existence
	subnets := []*string{}
	if conf.Subnets == nil || len(conf.Subnets) == 0 {
		defSubnet := findDefaultSubnet(ctx, sess, vpc)
		if defSubnet == nil {
			return errors.New("There is no default subnet")
		}
		subnets = append(subnets, defSubnet)
	} else {
		for _, arg := range conf.Subnets {
			for _, subnet := range strings.Split(aws.StringValue(arg), ",") {
				subnets = append(subnets, aws.String(subnet))
			}
		}
	}
	conf.Subnets = subnets

	// Ensure security-group existence
	sgs := []*string{}
	if conf.SecurityGroups == nil || len(conf.SecurityGroups) == 0 {
		defSecurityGroup := findDefaultSg(ctx, sess, vpc)
		if defSecurityGroup == nil {
			return errors.New("There is no default security group")
		}
		sgs = append(sgs, defSecurityGroup)
	} else {
		for _, arg := range conf.SecurityGroups {
			for _, sg := range strings.Split(aws.StringValue(arg), ",") {
				sgs = append(sgs, aws.String(sg))
			}
		}
	}
	conf.SecurityGroups = sgs
	return nil
}

func createClusterIfNotExist(ctx context.Context, sess *session.Session, cluster *string) error {
	out, err := ecs.New(sess).DescribeClustersWithContext(ctx, &ecs.DescribeClustersInput{
		Clusters: []*string{cluster},
	})
	if err != nil {
		return err
	}
	if len(out.Clusters) == 0 {
		_, err = ecs.New(sess).CreateClusterWithContext(ctx, &ecs.CreateClusterInput{
			ClusterName: cluster,
		})
	}
	return err
}

func findDefaultVPC(ctx context.Context, sess *session.Session) *string {
	out, err := ec2.New(sess).DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil || len(out.Vpcs) == 0 {
		return nil
	}
	return out.Vpcs[0].VpcId
}

func findDefaultSubnet(ctx context.Context, sess *session.Session, vpc *string) *string {
	out, err := ec2.New(sess).DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpc},
			},
			&ec2.Filter{
				Name:   aws.String("default-for-az"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil || len(out.Subnets) == 0 {
		return nil
	}
	return out.Subnets[0].SubnetId
}

func findDefaultSg(ctx context.Context, sess *session.Session, vpc *string) *string {
	out, err := ec2.New(sess).DescribeSecurityGroupsWithContext(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpc},
			},
			&ec2.Filter{
				Name:   aws.String("group-name"),
				Values: []*string{aws.String("default")},
			},
		},
	})
	if err != nil || len(out.SecurityGroups) == 0 {
		return nil
	}
	return out.SecurityGroups[0].GroupId
}

const (
	ecsExecutionPolicyArn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
	fargate               = "FARGATE"
	awsVPC                = "awsvpc"
	awsCWLogs             = "awslogs"
)

func createResouces(ctx context.Context, sess *session.Session, conf *Config, id string, image *string) (*string, *string, error) {
	// Make a temporary log group
	if err := createLogGroup(ctx, sess, conf, id); err != nil {
		return nil, nil, err
	}
	// Make a temporary IAM role
	role, err := createIAMRole(ctx, sess, conf, id)
	if err != nil {
		return nil, nil, err
	}
	// Make a temporary task definition
	task, err := registerTaskDef(ctx, sess, conf, id, image, aws.StringValue(role))
	if err != nil {
		return nil, nil, err
	}
	return task, role, nil
}

func createLogGroup(ctx context.Context, sess *session.Session, conf *Config, id string) error {
	_, err := cw.New(sess).CreateLogGroupWithContext(ctx, &cw.CreateLogGroupInput{
		LogGroupName: aws.String(fmt.Sprintf("/ecs/%s", id)),
	})
	return err
}

func createIAMRole(ctx context.Context, sess *session.Session, conf *Config, id string) (*string, error) {
	out, err := iam.New(sess).CreateRoleWithContext(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(id),
		AssumeRolePolicyDocument: aws.String(`{
  "Statement": [{
    "Effect": "Allow",
    "Action": "sts:AssumeRole",
    "Principal": {
      "Service": "ecs-tasks.amazonaws.com"
    }
  }]
}`),
		Path: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}
	if _, err = iam.New(sess).AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(id),
		PolicyArn: aws.String(ecsExecutionPolicyArn),
	}); err != nil {
		return nil, err
	}
	return out.Role.Arn, nil
}

func registerTaskDef(ctx context.Context, sess *session.Session, conf *Config, id string, image *string, role string) (*string, error) {
	environments := []*ecs.KeyValuePair{}
	for key, val := range conf.Environments {
		environments = append(environments, &ecs.KeyValuePair{
			Name:  aws.String(key),
			Value: val,
		})
	}
	input := ecs.RegisterTaskDefinitionInput{
		Family:                  conf.TaskDefFamily,
		RequiresCompatibilities: []*string{aws.String(fargate)},
		ExecutionRoleArn:        aws.String(role),
		TaskRoleArn:             conf.TaskRoleArn,
		Cpu:                     conf.CPU,
		Memory:                  conf.Memory,
		NetworkMode:             aws.String(awsVPC),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			&ecs.ContainerDefinition{
				Name:         aws.String("app"),
				Image:        image,
				EntryPoint:   conf.Entrypoint,
				Command:      conf.Commands,
				Environment:  environments,
				DockerLabels: conf.Labels,
				Essential:    aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String(awsCWLogs),
					Options: map[string]*string{
						"awslogs-region":        conf.AwsRegion,
						"awslogs-group":         aws.String(fmt.Sprintf("/ecs/%s", id)),
						"awslogs-stream-prefix": aws.String("fargate"),
					},
				},
			},
		},
	}
	if conf.IsDebugMode {
		lib.PrintJSON(input)
	}
	out, err := ecs.New(sess).RegisterTaskDefinitionWithContext(ctx, &input)
	if err != nil {
		return nil, err
	}
	return out.TaskDefinition.TaskDefinitionArn, nil
}

func run(ctx context.Context, sess *session.Session, conf *Config, taskARN *string, id string) ([]*ecs.Task, error) {
	input := ecs.RunTaskInput{
		Cluster:        conf.EcsCluster,
		LaunchType:     aws.String(fargate),
		TaskDefinition: taskARN,
		Count:          conf.NumberOfTasks,
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String("ENABLED"),
				Subnets:        conf.Subnets,
				SecurityGroups: conf.SecurityGroups,
			},
		},
	}
	if conf.IsDebugMode {
		lib.PrintJSON(input)
	}
	// Avoid the following error
	// ClientException: ECS was unable to assume the role that was provided for this task.
	timeout := time.After(30 * time.Second)
	for {
		var err error
		select {
		case <-timeout:
			return nil, errors.New("The execute role for this task was not in Active in 30sec")
		default:
			var out *ecs.RunTaskOutput
			out, err = ecs.New(sess).RunTaskWithContext(ctx, &input)
			if err == nil {
				return out.Tasks, nil
			}
			if ae, ok := err.(awserr.Error); ok && strings.EqualFold(ae.Code(), ecs.ErrCodeClientException) {
				time.Sleep(1 * time.Second)
				continue
			}
			return nil, err
		}
	}
}

func waitForTaskDone(ctx context.Context, sess *session.Session, conf *Config, tasks []*ecs.Task) (*int64, error) {
	timeout := time.After(time.Duration(aws.Int64Value(conf.TaskTimeout)) * time.Minute)
	taskARNs := []*string{}
	for _, task := range tasks {
		taskARNs = append(taskARNs, task.TaskArn)
	}
	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("The task did not finish in %d minutes", aws.Int64Value(conf.TaskTimeout))
		default:
			tasks, err := ecs.New(sess).DescribeTasksWithContext(ctx, &ecs.DescribeTasksInput{
				Cluster: conf.EcsCluster,
				Tasks:   taskARNs,
			})
			if err != nil {
				return nil, err
			}
			if len(tasks.Tasks) > 0 {
				done := true
				for _, task := range tasks.Tasks {
					done = done && strings.EqualFold(aws.StringValue(task.LastStatus), "STOPPED")
				}
				if done {
					if conf.IsDebugMode {
						lib.PrintJSON(tasks.Tasks)
					}
					if len(tasks.Tasks) > 0 && len(tasks.Tasks[0].Containers) > 0 {
						return tasks.Tasks[0].Containers[0].ExitCode, nil
					}
					return aws.Int64(-1), nil
				}
			}
			time.Sleep(5 * time.Second)
		}
	}
}

var regTaskID = regexp.MustCompile("task/(.*)")

func retrieveLogs(ctx context.Context, sess *session.Session, id string, tasks []*ecs.Task) map[string][]*cw.OutputLogEvent {
	result := map[string][]*cw.OutputLogEvent{}

	for _, task := range tasks {
		taskID := ""
		matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
		if len(matched) > 0 && len(matched[0]) > 1 {
			taskID = matched[0][1]
		}
		out, err := cw.New(sess).GetLogEventsWithContext(ctx, &cw.GetLogEventsInput{
			LogGroupName:  aws.String(fmt.Sprintf("/ecs/%s", id)),
			LogStreamName: aws.String(fmt.Sprintf("fargate/app/%s", taskID)),
		})
		if err == nil {
			result[taskID] = out.Events
		}
	}
	return result
}

// DeleteResouces deletes temporary AWS resources
func DeleteResouces(conf *Config) {
	sess, _ := lib.Session(conf.AwsAccessKey, conf.AwsSecretKey, conf.AwsRegion, nil) // nolint

	// Delete the temporary task definition
	deregisterTaskDef(sess, taskARN)

	// Delete the temporary IAM role
	deleteIAMRole(sess, resourceID)

	// Delete the temporary log group
	deleteLogGroup(sess, resourceID)

	// Delete the temporary ECS cluster
	deleteECSCluster(sess, resourceID)
}

func deregisterTaskDef(sess *session.Session, taskARN *string) {
	ecs.New(sess).DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{ // nolint
		TaskDefinition: taskARN,
	})
}

func deleteIAMRole(sess *session.Session, id string) {
	iam.New(sess).DetachRolePolicy(&iam.DetachRolePolicyInput{ // nolint
		RoleName:  aws.String(id),
		PolicyArn: aws.String(ecsExecutionPolicyArn),
	})
	iam.New(sess).DeleteRole(&iam.DeleteRoleInput{ // nolint
		RoleName: aws.String(id),
	})
}

func deleteLogGroup(sess *session.Session, id string) {
	cw.New(sess).DeleteLogGroup(&cw.DeleteLogGroupInput{ // nolint
		LogGroupName: aws.String(fmt.Sprintf("/ecs/%s", id)),
	})
}

func deleteECSCluster(sess *session.Session, id string) {
	ecs.New(sess).DeleteCluster(&ecs.DeleteClusterInput{ // nolint
		Cluster: aws.String(id),
	})
}

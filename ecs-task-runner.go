package etr

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
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
	"golang.org/x/sync/errgroup"
)

// AwsConfig is set of AWS configurations
type AwsConfig struct {
	AccessKey *string
	SecretKey *string
	Region    *string
}

// CommonConfig is set of common configurations
type CommonConfig struct {
	EcsCluster     *string
	Timeout        *int64
	ExtendedOutput *bool
	IsDebugMode    bool
}

// RunConfig is set of configurations for running a container
type RunConfig struct {
	Aws            *AwsConfig
	Common         *CommonConfig
	Image          string
	ForceECR       *bool
	TaskDefFamily  *string
	Entrypoint     []*string
	Commands       []*string
	Ports          []*int64
	Environments   map[string]*string
	Labels         map[string]*string
	Subnets        []*string
	SecurityGroups []*string
	CPU            *string
	Memory         *string
	TaskRoleArn    *string
	ExecRoleName   *string
	NumberOfTasks  *int64
	AssignPublicIP *bool
	Asynchronous   *bool
}

// StopConfig is set of configurations for stopping a container
type StopConfig struct {
	Aws       *AwsConfig
	Common    *CommonConfig
	RequestID *string
	TaskARNs  []*string
}

var (
	requestID     string
	taskDefARN    *string
	exitNormally  *int64
	exitWithError *int64
)

func init() {
	requestID = fmt.Sprintf("ecs-task-runner-%s", uuid.New().String())
	exitNormally = aws.Int64(0)
	exitWithError = aws.Int64(1)
}

// Run runs the docker image on Amazon ECS
func Run(ctx context.Context, conf *RunConfig) (exitCode *int64, err error) {
	startedAt := time.Now()

	if conf.Common.IsDebugMode {
		lib.PrintJSON(conf)
	}
	// Check AWS credentials
	sess, err := lib.Session(conf.Aws.AccessKey, conf.Aws.SecretKey, conf.Aws.Region, nil)
	if err != nil {
		return exitWithError, err
	}
	eg, _ := errgroup.WithContext(context.Background())

	// Check existence of the image on ECR
	var image *string
	eg.Go(func() (err error) {
		image, err = validateImageName(ctx, conf, sess)
		return err
	})
	// Ensure resource existence
	eg.Go(func() (err error) {
		return ensureAWSResources(ctx, sess, conf)
	})
	if err = eg.Wait(); err != nil {
		return exitWithError, err
	}
	// Create AWS resources
	var taskDefInput *ecs.RegisterTaskDefinitionInput
	taskDefARN, _, taskDefInput, err = createResouces(ctx, sess, conf, image)
	if err != nil {
		DeleteResouces(conf.Aws, conf.Common)
		return exitWithError, err
	}
	// Run the ECS task
	runTaskAt := time.Now()
	tasks, runconfig, err := run(ctx, sess, conf)
	if err != nil {
		DeleteResouces(conf.Aws, conf.Common)
		return exitWithError, err
	}
	// Asynchronous job
	if aws.BoolValue(conf.Asynchronous) {
		// Wait for its start
		tasks, err = waitForTask(ctx, sess, conf.Common, tasks, func(task *ecs.Task) bool {
			return !strings.EqualFold(aws.StringValue(task.LastStatus), "PROVISIONING") &&
				!strings.EqualFold(aws.StringValue(task.LastStatus), "PENDING")
		})
		if err != nil {
			DeleteResouces(conf.Aws, conf.Common)
			return exitWithError, err
		}
		outputRunResults(ctx, conf, startedAt, runTaskAt, nil, nil, taskDefInput, runconfig, tasks)
		if len(tasks) == 0 || len(tasks[0].Containers) == 0 {
			return exitWithError, nil
		}
		return exitNormally, nil
	}
	// Wait for its done
	tasks, err = waitForTask(ctx, sess, conf.Common, tasks, func(task *ecs.Task) bool {
		// dont have to wait until its 'stopped'
		// return strings.EqualFold(aws.StringValue(task.LastStatus), "STOPPED")
		return task.ExecutionStoppedAt != nil
	})
	if err != nil {
		DeleteResouces(conf.Aws, conf.Common)
		return exitWithError, err
	}
	// Retrieve app log
	logs := retrieveLogs(ctx, sess, tasks)
	retrieveLogsAt := time.Now()

	// Delete AWS resources
	DeleteResouces(conf.Aws, conf.Common)

	// Format the result
	outputRunResults(ctx, conf, startedAt, runTaskAt, &retrieveLogsAt, logs, taskDefInput, runconfig, tasks)

	if len(tasks) == 0 || len(tasks[0].Containers) == 0 {
		return exitWithError, nil
	}
	exitCode = aws.Int64(0)
	for _, task := range tasks {
		for _, container := range task.Containers {
			if aws.Int64Value(exitCode) != 0 {
				break
			}
			exitCode = container.ExitCode
		}
	}
	return exitCode, nil
}

// Stop stops the Fargate container on Amazon ECS
func Stop(ctx context.Context, conf *StopConfig) (exitCode *int64, err error) {

	// Check AWS credentials
	sess, err := lib.Session(conf.Aws.AccessKey, conf.Aws.SecretKey, conf.Aws.Region, nil)
	if err != nil {
		return exitWithError, err
	}
	// Ensure parameters
	requestID = aws.StringValue(conf.RequestID)
	if conf.Common.EcsCluster == nil || aws.StringValue(conf.Common.EcsCluster) == "" {
		conf.Common.EcsCluster = conf.RequestID
	}
	if conf.Common.IsDebugMode {
		lib.PrintJSON(conf)
	}
	// Retrieve all tasks to check cluster can be deleted or not
	all, err := ecs.New(sess).ListTasksWithContext(ctx, &ecs.ListTasksInput{
		Cluster: conf.Common.EcsCluster,
	})
	if err != nil {
		return exitWithError, err
	}
	// Stop the task
	tasks := []*ecs.Task{}
	if len(conf.TaskARNs) == 0 {
		conf.TaskARNs = all.TaskArns
	}
	for _, taskARN := range conf.TaskARNs {
		task, err := stopTask(ctx, sess, conf.Common, taskARN)
		if err != nil {
			return exitWithError, err
		}
		taskDefARN = task.TaskDefinitionArn
		tasks = append(tasks, task)
	}
	tasks, _ = waitForTask(ctx, sess, conf.Common, tasks, func(task *ecs.Task) bool { // nolint
		return task.ExecutionStoppedAt != nil
	})
	outputStopResults(ctx, conf, retrieveLogs(ctx, sess, tasks), tasks)

	// Delete AWS resources
	if len(all.TaskArns) == len(tasks) {
		DeleteResouces(conf.Aws, conf.Common)
	}
	return aws.Int64(0), nil
}

func validateImageName(ctx context.Context, conf *RunConfig, sess *session.Session) (*string, error) {
	imageHost, imageName, imageTag, err := parseImageName(conf.Image)
	if err != nil {
		lib.Errors.Println("Provided image name is invalid.")
		return nil, err
	}
	// Try to make up ECR image name
	if aws.BoolValue(conf.ForceECR) {
		account, err := sts.New(sess).GetCallerIdentityWithContext(ctx, nil)
		if err != nil {
			return nil, errors.New("Provided AWS credentials are invalid")
		}
		if conf.Common.IsDebugMode {
			lib.PrintJSON(account)
		}
		if !strings.Contains(aws.StringValue(imageHost), aws.StringValue(account.Account)) {
			imageName = aws.String(fmt.Sprintf(
				"%s/%s",
				aws.StringValue(imageHost),
				aws.StringValue(imageName),
			))
			imageHost = aws.String(fmt.Sprintf(
				"%s.dkr.ecr.%s.amazonaws.com",
				account,
				aws.StringValue(conf.Aws.Region),
			))
		}
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
			"%s%s",
			aws.StringValue(imageName),
			aws.StringValue(imageTag),
		)), nil
	}
	return aws.String(fmt.Sprintf(
		"%s/%s%s",
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
	imageTag := ":latest"
	if candidate, ok := ref.(reference.Tagged); ok {
		imageTag = ":" + candidate.Tag()
	}
	if candidate, ok := ref.(reference.Digested); ok {
		digest := candidate.Digest()
		if digest.Validate() == nil {
			imageTag = "@" + digest.String()
		}
	}
	return aws.String(imageHost), aws.String(imageName), aws.String(imageTag), nil
}

func ensureAWSResources(ctx context.Context, sess *session.Session, conf *RunConfig) error {
	eg, _ := errgroup.WithContext(context.Background())
	vpc := findDefaultVPC(ctx, sess)

	// Ensure cluster existence
	eg.Go(func() error {
		if conf.Common.EcsCluster == nil || aws.StringValue(conf.Common.EcsCluster) == "" {
			conf.Common.EcsCluster = aws.String(requestID)
		}
		return createClusterIfNotExist(ctx, sess, conf.Common.EcsCluster)
	})

	// Ensure subnets existence
	eg.Go(func() (err error) {
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
		return nil
	})

	// Ensure security-group existence
	eg.Go(func() (err error) {
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
	})
	return eg.Wait()
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

func createResouces(ctx context.Context, sess *session.Session, conf *RunConfig, image *string) (task *string, role *string, taskDefInput *ecs.RegisterTaskDefinitionInput, e error) {
	eg, _ := errgroup.WithContext(context.Background())

	eg.Go(func() error {
		// Make a temporary log group
		return createLogGroup(ctx, sess, conf)
	})
	eg.Go(func() (err error) {
		// Make a temporary IAM role
		role, err = createIAMRole(ctx, sess, conf)
		if err != nil {
			return err
		}
		// Make a temporary task definition
		taskDefARN, taskDefInput, err = registerTaskDef(ctx, sess, conf, image, aws.StringValue(role))
		return
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, nil, err
	}
	return taskDefARN, role, taskDefInput, nil
}

func createLogGroup(ctx context.Context, sess *session.Session, conf *RunConfig) error {
	_, err := cw.New(sess).CreateLogGroupWithContext(ctx, &cw.CreateLogGroupInput{
		LogGroupName: aws.String(fmt.Sprintf("/ecs/%s", requestID)),
	})
	return err
}

func createIAMRole(ctx context.Context, sess *session.Session, conf *RunConfig) (*string, error) {
	role, err := iam.New(sess).GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: conf.ExecRoleName,
	})
	if err == nil && role.Role != nil {
		// TODO Ensure ecsExecutionPolicyArn is attached
		return role.Role.Arn, nil
	}
	out, err := iam.New(sess).CreateRoleWithContext(ctx, &iam.CreateRoleInput{
		RoleName: conf.ExecRoleName,
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
		RoleName:  conf.ExecRoleName,
		PolicyArn: aws.String(ecsExecutionPolicyArn),
	}); err != nil {
		return nil, err
	}
	return out.Role.Arn, nil
}

func registerTaskDef(ctx context.Context, sess *session.Session, conf *RunConfig, image *string, role string) (*string, *ecs.RegisterTaskDefinitionInput, error) {
	environments := []*ecs.KeyValuePair{}
	for key, val := range conf.Environments {
		environments = append(environments, &ecs.KeyValuePair{
			Name:  aws.String(key),
			Value: val,
		})
	}
	ports := []*ecs.PortMapping{}
	for _, port := range conf.Ports {
		ports = append(ports, &ecs.PortMapping{
			ContainerPort: port,
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
				PortMappings: ports,
				DockerLabels: conf.Labels,
				Essential:    aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String(awsCWLogs),
					Options: map[string]*string{
						"awslogs-region":        conf.Aws.Region,
						"awslogs-group":         aws.String(fmt.Sprintf("/ecs/%s", requestID)),
						"awslogs-stream-prefix": aws.String("fargate"),
					},
				},
			},
		},
	}
	if conf.Common.IsDebugMode {
		lib.PrintJSON(input)
	}
	out, err := ecs.New(sess).RegisterTaskDefinitionWithContext(ctx, &input)
	if err != nil {
		return nil, nil, err
	}
	return out.TaskDefinition.TaskDefinitionArn, &input, nil
}

func run(ctx context.Context, sess *session.Session, conf *RunConfig) ([]*ecs.Task, *ecs.RunTaskInput, error) {
	assignPublicIP := "ENABLED"
	if !aws.BoolValue(conf.AssignPublicIP) {
		assignPublicIP = "DISABLED"
	}
	input := ecs.RunTaskInput{
		Cluster:        conf.Common.EcsCluster,
		LaunchType:     aws.String(fargate),
		TaskDefinition: taskDefARN,
		Count:          conf.NumberOfTasks,
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String(assignPublicIP),
				Subnets:        conf.Subnets,
				SecurityGroups: conf.SecurityGroups,
			},
		},
	}
	if conf.Common.IsDebugMode {
		lib.PrintJSON(input)
	}
	// Avoid the following error
	// ClientException: ECS was unable to assume the role that was provided for this task.
	timeout := time.After(30 * time.Second)
	for {
		var err error
		select {
		case <-timeout:
			return nil, nil, errors.New("The execute role for this task was not in Active in 30sec")
		default:
			var out *ecs.RunTaskOutput
			out, err = ecs.New(sess).RunTaskWithContext(ctx, &input)
			if err == nil {
				return out.Tasks, &input, nil
			}
			if ae, ok := err.(awserr.Error); ok && strings.EqualFold(ae.Code(), ecs.ErrCodeClientException) {
				time.Sleep(1 * time.Second)
				continue
			}
			return nil, nil, err
		}
	}
}

type judgeFunc func(task *ecs.Task) bool

func waitForTask(ctx context.Context, sess *session.Session, conf *CommonConfig, tasks []*ecs.Task, judge judgeFunc) ([]*ecs.Task, error) {
	timeout := time.After(time.Duration(aws.Int64Value(conf.Timeout)) * time.Minute)
	taskARNs := []*string{}
	for _, task := range tasks {
		taskARNs = append(taskARNs, task.TaskArn)
	}
	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("The task did not finish in %d minutes", aws.Int64Value(conf.Timeout))
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
					done = done && judge(task)
				}
				if done {
					if conf.IsDebugMode {
						lib.PrintJSON(tasks.Tasks)
					}
					return tasks.Tasks, nil
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func stopTask(ctx context.Context, sess *session.Session, conf *CommonConfig, taskARN *string) (*ecs.Task, error) {
	task, err := ecs.New(sess).StopTaskWithContext(ctx, &ecs.StopTaskInput{
		Cluster: conf.EcsCluster,
		Task:    taskARN,
	})
	if err != nil {
		return nil, err
	}
	return task.Task, nil
}

var regTaskID = regexp.MustCompile("task/(.*)")

func retrieveLogs(ctx context.Context, sess *session.Session, tasks []*ecs.Task) map[string][]*cw.OutputLogEvent {
	result := map[string][]*cw.OutputLogEvent{}

	for _, task := range tasks {
		taskID := ""
		matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
		if len(matched) > 0 && len(matched[0]) > 1 {
			taskID = matched[0][1]
		}
		out, err := cw.New(sess).GetLogEventsWithContext(ctx, &cw.GetLogEventsInput{
			LogGroupName:  aws.String(fmt.Sprintf("/ecs/%s", requestID)),
			LogStreamName: aws.String(fmt.Sprintf("fargate/app/%s", taskID)),
		})
		if err == nil {
			result[aws.StringValue(task.TaskArn)] = out.Events
		}
	}
	return result
}

// DeleteResouces deletes temporary AWS resources
func DeleteResouces(aws *AwsConfig, conf *CommonConfig) {
	sess, err := lib.Session(aws.AccessKey, aws.SecretKey, aws.Region, nil)
	if err != nil {
		return
	}
	wg := sync.WaitGroup{}
	wg.Add(3)

	// Delete the temporary task definition
	go func() {
		defer wg.Done()
		deregisterTaskDef(sess, conf.IsDebugMode)
	}()
	// Delete the temporary log group
	go func() {
		defer wg.Done()
		deleteLogGroup(sess, conf.IsDebugMode)
	}()
	// Delete the temporary ECS cluster
	go func() {
		defer wg.Done()
		deleteECSCluster(sess, conf.IsDebugMode)
	}()
	wg.Wait()
}

func deregisterTaskDef(sess *session.Session, debug bool) {
	if _, err := ecs.New(sess).DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: taskDefARN,
	}); err != nil && debug {
		lib.PrintJSON(err)
	}
}

func deleteLogGroup(sess *session.Session, debug bool) {
	if _, err := cw.New(sess).DeleteLogGroup(&cw.DeleteLogGroupInput{
		LogGroupName: aws.String(fmt.Sprintf("/ecs/%s", requestID)),
	}); err != nil && debug {
		lib.PrintJSON(err)
	}
}

func deleteECSCluster(sess *session.Session, debug bool) {
	if _, err := ecs.New(sess).DeleteCluster(&ecs.DeleteClusterInput{
		Cluster: aws.String(requestID),
	}); err != nil && debug {
		lib.PrintJSON(err)
	}
}

func outputRunResults(ctx context.Context, conf *RunConfig, startedAt, runTaskAt time.Time, logsAt *time.Time, logs map[string][]*cw.OutputLogEvent, taskdef *ecs.RegisterTaskDefinitionInput, runconfig *ecs.RunTaskInput, tasks []*ecs.Task) {
	result := map[string]interface{}{}

	if aws.BoolValue(conf.Asynchronous) { // Async mode
		result["RequestID"] = requestID
		if len(tasks) > 0 {
			resources := []map[string]string{}
			for _, task := range tasks {
				resource := map[string]string{}
				resource["TaskARN"] = aws.StringValue(task.TaskArn)
				resource["PublicIP"] = retrievePublicIP(ctx, conf.Aws, task, conf.Common.IsDebugMode)
				resources = append(resources, resource)
			}
			result["Tasks"] = resources
		}
	} else { // Sync mode
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
	}
	if aws.BoolValue(conf.Common.ExtendedOutput) {
		timelines := []map[string]string{}
		resources := []map[string]interface{}{}
		if len(tasks) > 0 {
			for _, task := range tasks {
				resource := map[string]interface{}{}
				container := taskdef.ContainerDefinitions[0]
				resource["ClusterArn"] = aws.StringValue(task.ClusterArn)
				resource["TaskDefinitionArn"] = aws.StringValue(task.TaskDefinitionArn)
				resource["TaskArn"] = aws.StringValue(task.TaskArn)
				resource["TaskRoleArn"] = aws.StringValue(taskdef.TaskRoleArn)
				resource["LogGroup"] = aws.StringValue(container.LogConfiguration.Options["awslogs-group"])
				resource["PublicIP"] = retrievePublicIP(ctx, conf.Aws, task, conf.Common.IsDebugMode)

				containers := []map[string]string{}
				taskID := ""
				matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
				if len(matched) > 0 && len(matched[0]) > 1 {
					taskID = matched[0][1]
				}
				for _, c := range task.Containers {
					containers = append(containers, map[string]string{
						"ContainerArn": aws.StringValue(c.ContainerArn),
						"LogStream":    fmt.Sprintf("fargate/app/%s", taskID),
					})
				}
				resource["Containers"] = containers
				resources = append(resources, resource)

				timeline := map[string]string{}
				timeline["0"] = fmt.Sprintf("AppStartedAt:              %s", rfc3339(startedAt))
				timeline["1"] = fmt.Sprintf("AppTriedToRunFargateAt:    %s", rfc3339(runTaskAt))
				timeline["2"] = fmt.Sprintf("FargateCreatedAt:          %s", toStr(task.CreatedAt))
				timeline["3"] = fmt.Sprintf("FargatePullStartedAt:      %s", toStr(task.PullStartedAt))
				timeline["4"] = fmt.Sprintf("FargatePullStoppedAt:      %s", toStr(task.PullStoppedAt))
				timeline["5"] = fmt.Sprintf("FargateStartedAt:          %s", toStr(task.StartedAt))
				timeline["6"] = fmt.Sprintf("FargateExecutionStoppedAt: %s", toStr(task.ExecutionStoppedAt))
				timeline["7"] = fmt.Sprintf("FargateStoppedAt:          %s", toStr(task.StoppedAt))
				timeline["8"] = fmt.Sprintf("AppRetrievedLogsAt:        %s", rfc3339(aws.TimeValue(logsAt)))
				timeline["9"] = fmt.Sprintf("AppFinishedAt:             %s", rfc3339(time.Now()))
				timelines = append(timelines, timeline)
			}
		}
		result["meta"] = map[string]interface{}{
			"1.taskdef":   taskdef,
			"2.runconfig": runconfig,
			"3.resources": resources,
			"4.timeline":  timelines,
		}
	}
	lib.PrintJSON(result)
}

func outputStopResults(ctx context.Context, conf *StopConfig, logs map[string][]*cw.OutputLogEvent, tasks []*ecs.Task) {
	result := map[string]interface{}{}
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
	if aws.BoolValue(conf.Common.ExtendedOutput) {
		timelines := []map[string]string{}
		resources := []map[string]interface{}{}
		if len(tasks) > 0 {
			for _, task := range tasks {
				resource := map[string]interface{}{}
				resource["ClusterArn"] = aws.StringValue(task.ClusterArn)
				resource["TaskDefinitionArn"] = aws.StringValue(task.TaskDefinitionArn)
				resource["TaskArn"] = aws.StringValue(task.TaskArn)

				containers := []map[string]string{}
				taskID := ""
				matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
				if len(matched) > 0 && len(matched[0]) > 1 {
					taskID = matched[0][1]
				}
				for _, c := range task.Containers {
					containers = append(containers, map[string]string{
						"ContainerArn": aws.StringValue(c.ContainerArn),
						"LogStream":    fmt.Sprintf("fargate/app/%s", taskID),
					})
				}
				resource["Containers"] = containers
				resources = append(resources, resource)

				timeline := map[string]string{}
				timeline["1"] = fmt.Sprintf("FargateCreatedAt:          %s", toStr(task.CreatedAt))
				timeline["2"] = fmt.Sprintf("FargatePullStartedAt:      %s", toStr(task.PullStartedAt))
				timeline["3"] = fmt.Sprintf("FargatePullStoppedAt:      %s", toStr(task.PullStoppedAt))
				timeline["4"] = fmt.Sprintf("FargateStartedAt:          %s", toStr(task.StartedAt))
				timeline["5"] = fmt.Sprintf("FargateExecutionStoppedAt: %s", toStr(task.ExecutionStoppedAt))
				timeline["6"] = fmt.Sprintf("FargateStoppedAt:          %s", toStr(task.StoppedAt))
				timeline["7"] = fmt.Sprintf("AppFinishedAt:             %s", rfc3339(time.Now()))
				timelines = append(timelines, timeline)
			}
		}
		result["meta"] = map[string]interface{}{
			"1.resources": resources,
			"2.timeline":  timelines,
		}
	}
	lib.PrintJSON(result)
}

func toStr(t *time.Time) string {
	if t == nil {
		return "---"
	}
	return rfc3339(aws.TimeValue(t))
}

func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func retrievePublicIP(ctx context.Context, conf *AwsConfig, task *ecs.Task, debug bool) string {
	if task == nil || len(task.Attachments) == 0 {
		return ""
	}
	var eniID *string
	for _, detail := range task.Attachments[0].Details {
		if strings.EqualFold(aws.StringValue(detail.Name), "networkInterfaceId") {
			eniID = detail.Value
		}
	}
	if eniID == nil {
		return ""
	}
	sess, err := lib.Session(conf.AccessKey, conf.SecretKey, conf.Region, nil)
	if err != nil && debug {
		lib.PrintJSON(err)
		return ""
	}
	input := &ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []*string{eniID},
	}
	eni, err := ec2.New(sess).DescribeNetworkInterfacesWithContext(ctx, input)
	if err != nil {
		if debug {
			lib.PrintJSON(err)
		}
		return ""
	}
	if len(eni.NetworkInterfaces) == 0 {
		return ""
	}
	return aws.StringValue(eni.NetworkInterfaces[0].Association.PublicIp)
}

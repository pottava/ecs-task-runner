package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	commands "github.com/pottava/ecs-task-runner"
	"github.com/pottava/ecs-task-runner/conf"
	lib "github.com/pottava/ecs-task-runner/internal/aws"
	"github.com/pottava/ecs-task-runner/internal/log"
	cli "gopkg.in/alecthomas/kingpin.v2"
)

// for compile flags
var (
	ver    = "dev"
	commit string
	date   string
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			if os.Getenv("APP_DEBUG") == "1" {
				debug.PrintStack()
			}
			log.Errors.Fatal(err)
		}
	}()

	app := cli.New("ecs-rask-runner", "A synchronous task runner AWS Fargate on Amazon ECS")
	if len(commit) > 0 && len(date) > 0 {
		app.Version(fmt.Sprintf("%s-%s (built at %s)", ver, commit, date))
	} else {
		app.Version(ver)
	}
	// global flags
	awsconf := &conf.AwsConfig{}
	awsconf.AccessKey = app.Flag("access-key", "AWS access key ID.").
		Short('a').Envar("AWS_ACCESS_KEY_ID").String()
	awsconf.SecretKey = app.Flag("secret-key", "AWS secret access key.").
		Short('s').Envar("AWS_SECRET_ACCESS_KEY").String()
	awsconf.Profile = app.Flag("profile", "AWS default profile.").
		Envar("AWS_PROFILE").String()
	awsconf.AssumeRole = app.Flag("assume-role", "IAM Role ARN to be assumed.").
		Envar("AWS_ASSUME_ROLE").String()
	awsconf.MfaSerialNumber = app.Flag("mfa-serial-num", "A serial number of MFA device.").
		Envar("AWS_MFA_SERIAL_NUMBER").String()
	awsconf.MfaToken = app.Flag("mfa-token", "A token for MFA.").
		Envar("AWS_MFA_TOKEN").String()
	awsconf.Region = app.Flag("region", "AWS default region.").
		Short('r').Envar("AWS_DEFAULT_REGION").Default("us-east-1").String()

	common := &conf.CommonConfig{}
	common.AppVersion = ver
	if len(commit) > 0 && len(date) > 0 {
		common.AppVersion = fmt.Sprintf("%s-%s (built at %s)", ver, commit, date)
	}
	common.EcsCluster = app.Flag("cluster", "Amazon ECS cluster name.").
		Short('c').Envar("ECS_CLUSTER").String()
	common.ExecRoleName = app.Flag("exec-role-name", "Name of an execution role for the task.").
		Envar("EXEC_ROLE_NAME").Default("ecs-task-runner").String()
	common.Timeout = app.Flag("timeout", "Timeout minutes for the task.").
		Short('t').Envar("TASK_TIMEOUT").Default("30").Int64()
	common.ExtendedOutput = app.Flag("extended-output", "If it's True, meta data returns as well.").
		Envar("EXTENDED_OUTPUT").Default("false").Bool()
	common.IsDebugMode = os.Getenv("APP_DEBUG") == "1"

	// commands
	runconf := &conf.RunConfig{}
	runconf.Aws = awsconf
	runconf.Common = common
	run := app.Command("run", "Run a docker image as a Fargate container on ECS cluster.")
	image := run.Arg("image", "Docker image name to be executed on ECS.").
		Envar("DOCKER_IMAGE").Required().String()
	runconf.Spot = run.Flag("spot", "If it's True, fargate spot will be used.").
		Envar("FARGATE_SPOT").Default("false").Bool()
	runconf.ForceECR = run.Flag("force-ecr", "If it's True, you can use the shortened image name.").
		Short('f').Envar("FORCE_ECR").Default("false").Bool()
	runconf.TaskDefFamily = run.Flag("taskdef-family", "ECS Task Definition family name.").
		Envar("TASKDEF_FAMILY").Default("ecs-task-runner").String()
	entrypoints := run.Flag("entrypoint", "Override `ENTRYPOINT` of the image.").
		Envar("ENTRYPOINT").Strings()
	runconf.User = run.Flag("docker-user", "The user name to use inside the container.").
		Envar("USER").String()
	cmds := run.Flag("command", "Override `CMD` of the image.").
		Envar("COMMAND").Strings()
	subnets := run.Flag("subnets", "Subnets on where Fargate containers run.").
		Envar("SUBNETS").Strings()
	ports := run.Flag("port", "Publish ports.").
		Short('p').Envar("PORT").Int64List()
	envs := run.Flag("environment", "Add `ENV` to the container.").
		Short('e').Envar("ENVIRONMENT").Strings()
	labels := run.Flag("label", "Add `LABEL` to the container.").
		Short('l').Envar("LABEL").Strings()
	securityGroups := run.Flag("security-groups", "SecurityGroups to be assigned to containers.").
		Envar("SECURITY_GROUPS").Strings()
	runconf.CPU = run.Flag("cpu", "Requested vCPU to run Fargate containers.").
		Envar("CPU").Default("256").String()
	runconf.Memory = run.Flag("memory", "Requested memory to run Fargate containers.").
		Envar("MEMORY").Default("512").String()
	runconf.TaskRoleArn = run.Flag("task-role-arn", "ARN of an IAM Role for the task.").
		Envar("TASK_ROLE_ARN").String()
	runconf.NumberOfTasks = run.Flag("number", "Number of tasks.").
		Short('n').Envar("NUMBER").Default("1").Int64()
	runconf.KMSCustomKeyID = run.Flag("kms-key-id", "KMS custom key ID for SecretsManager decryption.").
		Envar("KMS_CUSTOMKEY_ID").String()
	runconf.DockerUser = run.Flag("user", "PrivateRegistry Username .").
		Envar("PRIVATE_REGISTRY_USER").String()
	runconf.DockerPassword = run.Flag("password", "PrivateRegistry Password.").
		Envar("PRIVATE_REGISTRY_PASSWORD").String()
	runconf.AssignPublicIP = run.Flag("assign-pub-ip", "If it's True, it assigns public IP.").
		Envar("ASSIGN_PUBLIC_IP").Default("true").Bool()
	runconf.ReadOnlyRootFS = run.Flag("readonly-rootfs", "If it's True, it makes the Root-FS READ only.").
		Envar("READONLY_ROOOTFS").Default("false").Bool()
	runconf.Asynchronous = run.Flag("async", "If it's True, the app does not wait for the job done.").
		Envar("ASYNC").Default("false").Bool()

	stopconf := &conf.StopConfig{}
	stopconf.Aws = awsconf
	stopconf.Common = common
	stop := app.Command("stop", "Stop a Fargate on ECS cluster.")
	stopconf.RequestID = stop.Arg("request-id", "Request ID.").
		Envar("REQUEST_ID").Required().String()
	taskARNs := stop.Flag("task-arn", "ECS Task ARN.").
		Envar("TASK_ARN").Strings()

	switch cli.MustParse(app.Parse(os.Args[1:])) {
	case run.FullCommand():
		runconf.Image = aws.StringValue(image)
		runconf.Entrypoint = []*string{}
		if entrypoints != nil {
			for _, candidate := range *entrypoints {
				for _, entrypoint := range strings.Split(candidate, ",") {
					runconf.Entrypoint = append(runconf.Entrypoint, aws.String(entrypoint))
				}
			}
		}
		runconf.Commands = []*string{}
		if cmds != nil {
			for _, candidate := range *cmds {
				for _, cmd := range strings.Split(candidate, ",") {
					runconf.Commands = append(runconf.Commands, aws.String(cmd))
				}
			}
		}
		runconf.Ports = []*int64{}
		if envs != nil {
			for _, candidate := range *ports {
				runconf.Ports = append(runconf.Ports, aws.Int64(candidate))
			}
		}
		runconf.Environments = map[string]*string{}
		if envs != nil {
			for _, candidate := range *envs {
				for _, env := range strings.Split(candidate, ",") {
					if keyval := strings.Split(env, "="); len(keyval) >= 2 {
						runconf.Environments[keyval[0]] = aws.String(strings.Join(keyval[1:], "="))
					}
				}
			}
		}
		runconf.Labels = map[string]*string{}
		if labels != nil {
			for _, candidate := range *labels {
				for _, label := range strings.Split(candidate, ",") {
					if keyval := strings.Split(label, "="); len(keyval) >= 2 {
						runconf.Labels[keyval[0]] = aws.String(strings.Join(keyval[1:], "="))
					}
				}
			}
		}
		runconf.Subnets = []*string{}
		if subnets != nil {
			for _, subnet := range *subnets {
				runconf.Subnets = append(runconf.Subnets, aws.String(subnet))
			}
		}
		runconf.SecurityGroups = []*string{}
		if securityGroups != nil {
			for _, securityGroup := range *securityGroups {
				runconf.SecurityGroups = append(runconf.SecurityGroups, aws.String(securityGroup))
			}
		}

		// Cancel
		ctx, cancel := context.WithCancel(context.Background())
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			cancel()
			if sess, err := lib.Session(runconf.Aws, runconf.Common.IsDebugMode); err == nil {
				commands.DeleteResouces(runconf.Aws, runconf.Common, sess)
			}
			os.Exit(1)
		}()

		// Execute
		out, err := commands.Run(ctx, runconf)
		if err != nil {
			log.Errors.Fatal(err)
			return
		}
		if aws.BoolValue(runconf.Asynchronous) {
			if aws.BoolValue(common.ExtendedOutput) {
				log.PrintJSON(struct {
					RequestID  string                     `json:"RequestID"`
					AsyncTasks []commands.OutputAsyncTask `json:"Tasks"`
					Meta       commands.OutputMeta        `json:"meta"`
				}{
					RequestID:  out.RequestID,
					AsyncTasks: out.AsyncTasks,
					Meta:       out.Meta,
				})
			} else {
				log.PrintJSON(struct {
					RequestID  string                     `json:"RequestID"`
					AsyncTasks []commands.OutputAsyncTask `json:"Tasks"`
				}{
					RequestID:  out.RequestID,
					AsyncTasks: out.AsyncTasks,
				})
			}
		} else {
			if aws.BoolValue(common.ExtendedOutput) {
				out.SyncLogs["meta"] = out.Meta
			}
			log.PrintJSON(out.SyncLogs)
		}
		os.Exit(int(aws.Int64Value(out.ExitCode)))

	case stop.FullCommand():
		stopconf.TaskARNs = []*string{}
		if taskARNs != nil {
			for _, taskARN := range *taskARNs {
				stopconf.TaskARNs = append(stopconf.TaskARNs, aws.String(taskARN))
			}
		}

		// Cancel
		ctx, cancel := context.WithCancel(context.Background())
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			cancel()
			os.Exit(1)
		}()

		// Execute
		out, err := commands.Stop(ctx, stopconf)
		if err != nil {
			log.Errors.Fatal(err)
			return
		}
		if aws.BoolValue(common.ExtendedOutput) {
			out.SyncLogs["meta"] = out.Meta
		}
		log.PrintJSON(out.SyncLogs)
		os.Exit(int(aws.Int64Value(out.ExitCode)))
	}
}

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
	log "github.com/pottava/ecs-task-runner/lib"
	cli "gopkg.in/alecthomas/kingpin.v2"
)

// for compile flags
var (
	version = "1.1.x"
	commit  string
	date    string
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
	if len(version) > 0 && len(date) > 0 {
		app.Version(fmt.Sprintf("%s-%s (built at %s)", version, commit, date))
	} else {
		app.Version(version)
	}
	// global flags
	conf := &commands.Config{}
	conf.AwsAccessKey = app.Flag("access-key", "AWS access key ID.").
		Short('a').Envar("AWS_ACCESS_KEY_ID").Required().String()
	conf.AwsSecretKey = app.Flag("secret-key", "AWS secret access key.").
		Short('s').Envar("AWS_SECRET_ACCESS_KEY").Required().String()
	conf.AwsRegion = app.Flag("region", "AWS region.").
		Short('r').Envar("AWS_DEFAULT_REGION").Default("us-east-1").String()

	// commands
	run := app.Command("run", "Run a docker image as a Fargate on ECS cluster.")
	conf.EcsCluster = run.Flag("cluster", "Amazon ECS cluster name.").
		Short('c').Envar("ECS_CLUSTER").String()
	image := run.Flag("image", "Docker image name to be executed on ECS.").
		Short('i').Envar("DOCKER_IMAGE").Required().String()
	conf.ForceECR = run.Flag("force-ecr", "If it's True, you can use the shortened image name.").
		Short('f').Envar("FORCE_ECR").Default("false").Bool()
	conf.TaskDefFamily = run.Flag("taskdef-family", "ECS Task Definition family name.").
		Envar("TASKDEF_FAMILY").Default("ecs-task-runner").String()
	entrypoints := run.Flag("entrypoint", "Override `ENTRYPOINT` of the image.").
		Envar("ENTRYPOINT").Strings()
	cmds := run.Flag("command", "Override `CMD` of the image.").
		Envar("COMMAND").Strings()
	subnets := run.Flag("subnets", "Subnets on where Fargate containers run.").
		Envar("SUBNETS").Strings()
	envs := run.Flag("environment", "Add `ENV` to the container.").
		Short('e').Envar("ENVIRONMENT").Strings()
	labels := run.Flag("label", "Add `LABEL` to the container.").
		Short('l').Envar("LABEL").Strings()
	securityGroups := run.Flag("security-groups", "SecurityGroups to be assigned to containers.").
		Envar("SECURITY_GROUPS").Strings()
	conf.CPU = run.Flag("cpu", "Requested vCPU to run Fargate containers.").
		Envar("CPU").Default("256").String()
	conf.Memory = run.Flag("memory", "Requested memory to run Fargate containers.").
		Envar("MEMORY").Default("512").String()
	conf.TaskRoleArn = run.Flag("task-role-arn", "ARN of an IAM Role for the task.").
		Envar("TASK_ROLE_ARN").String()
	conf.ExecRoleName = run.Flag("exec-role-name", "Name of an execution role for the task.").
		Envar("EXEC_ROLE_NAME").Default("ecs-task-runner").String()
	conf.NumberOfTasks = run.Flag("number", "Number of tasks.").
		Short('n').Envar("NUMBER").Default("1").Int64()
	conf.TaskTimeout = run.Flag("timeout", "Timeout minutes for the task.").
		Short('t').Envar("TASK_TIMEOUT").Default("30").Int64()
	conf.ExtendedOutput = run.Flag("extended-output", "If it's True, meta data returns as well.").
		Envar("EXTENDED_OUTPUT").Default("false").Bool()

	switch cli.MustParse(app.Parse(os.Args[1:])) {
	case run.FullCommand():
		conf.Image = aws.StringValue(image)
		conf.Entrypoint = []*string{}
		if entrypoints != nil {
			for _, candidate := range *entrypoints {
				for _, entrypoint := range strings.Split(candidate, ",") {
					conf.Entrypoint = append(conf.Entrypoint, aws.String(entrypoint))
				}
			}
		}
		conf.Commands = []*string{}
		if cmds != nil {
			for _, candidate := range *cmds {
				for _, cmd := range strings.Split(candidate, ",") {
					conf.Commands = append(conf.Commands, aws.String(cmd))
				}
			}
		}
		conf.Environments = map[string]*string{}
		if envs != nil {
			for _, candidate := range *envs {
				for _, env := range strings.Split(candidate, ",") {
					if keyval := strings.Split(env, "="); len(keyval) >= 2 {
						conf.Environments[keyval[0]] = aws.String(strings.Join(keyval[1:], "="))
					}
				}
			}
		}
		conf.Labels = map[string]*string{}
		if labels != nil {
			for _, candidate := range *labels {
				for _, label := range strings.Split(candidate, ",") {
					if keyval := strings.Split(label, "="); len(keyval) >= 2 {
						conf.Labels[keyval[0]] = aws.String(strings.Join(keyval[1:], "="))
					}
				}
			}
		}
		conf.Subnets = []*string{}
		if subnets != nil {
			for _, subnet := range *subnets {
				conf.Subnets = append(conf.Subnets, aws.String(subnet))
			}
		}
		conf.SecurityGroups = []*string{}
		if securityGroups != nil {
			for _, securityGroup := range *securityGroups {
				conf.SecurityGroups = append(conf.SecurityGroups, aws.String(securityGroup))
			}
		}
		conf.IsDebugMode = os.Getenv("APP_DEBUG") == "1"

		// Cancel
		ctx, cancel := context.WithCancel(context.Background())
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			cancel()
			commands.DeleteResouces(conf)
			os.Exit(1)
		}()

		// Execute
		exitCode, err := commands.Run(ctx, conf)
		if err != nil {
			if conf.IsDebugMode {
				debug.PrintStack()
			}
			log.Errors.Fatal(err)
			return
		}
		os.Exit(int(aws.Int64Value(exitCode)))
	}
}

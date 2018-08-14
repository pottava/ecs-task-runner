package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/aws/aws-sdk-go/aws"
	commands "github.com/pottava/ecs-task-runner"
	log "github.com/pottava/ecs-task-runner/lib"
	cli "gopkg.in/alecthomas/kingpin.v2"
)

// for compile flags
var (
	version = "dev"
	commit  string
	date    string
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			debug.PrintStack()
			log.Logger.Fatal(err)
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
	conf.AwsAccessKey = app.Flag("access_key", "AWS access key ID.").
		Short('a').Envar("AWS_ACCESS_KEY_ID").Required().String()
	conf.AwsSecretKey = app.Flag("secret_key", "AWS secret access key.").
		Short('s').Envar("AWS_SECRET_ACCESS_KEY").Required().String()
	conf.AwsRegion = app.Flag("region", "AWS region.").
		Short('r').Envar("AWS_DEFAULT_REGION").Default("us-east-1").String()

	// commands
	run := app.Command("run", "Run a docker image as a Fargate on ECS cluster.")
	conf.EcsCluster = run.Flag("cluster", "Amazon ECS cluster name.").
		Short('c').Envar("ECS_CLUSTER").String()
	image := run.Flag("image", "Docker image name to be executed on ECS.").
		Short('i').Envar("DOCKER_IMAGE").Required().String()
	entrypoints := run.Flag("entrypoint", "Override `ENTRYPOINT` of the image.").
		Envar("ENTRYPOINT").Strings()
	cmds := run.Flag("command", "Override `CMD` of the image.").
		Envar("COMMAND").Strings()
	subnets := run.Flag("subnets", "Subnets on where Fargate containers run.").
		Envar("SUBNETS").Strings()
	securityGroups := run.Flag("security_groups", "SecurityGroups to be assigned to containers.").
		Envar("SECURITY_GROUPS").Strings()
	conf.CPU = run.Flag("cpu", "Requested vCPU to run Fargate containers.").
		Envar("CPU").Default("256").String()
	conf.Memory = run.Flag("memory", "Requested memory to run Fargate containers.").
		Envar("MEMORY").Default("512").String()
	conf.TaskRoleArn = run.Flag("role", "ARN of an IAM Role for the task.").
		Envar("TASK_ROLE").String()
	conf.NumberOfTasks = run.Flag("number", "Number of tasks.").
		Short('n').Envar("NUMBER").Default("1").Int64()
	conf.TaskTimeout = run.Flag("timeout", "Timeout minutes for the task.").
		Short('t').Envar("TASK_TIMEOUT").Default("30").Int64()

	switch cli.MustParse(app.Parse(os.Args[1:])) {
	case run.FullCommand():
		conf.Image = aws.StringValue(image)
		conf.Entrypoint = []*string{}
		if entrypoints != nil {
			for _, entrypoint := range *entrypoints {
				conf.Entrypoint = append(conf.Entrypoint, aws.String(entrypoint))
			}
		}
		conf.Commands = []*string{}
		if cmds != nil {
			for _, cmd := range *cmds {
				conf.Commands = append(conf.Commands, aws.String(cmd))
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
		if err := commands.Run(conf); err != nil {
			log.Logger.Fatal(err)
			return
		}
	}
}

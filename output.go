package etr

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	cw "github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pottava/ecs-task-runner/lib"
)

var (
	exitNormally  *int64
	exitWithError *int64
)

func init() {
	exitNormally = aws.Int64(0)
	exitWithError = aws.Int64(1)
}

// Output is the result of this application
type Output struct {
	ExitCode   *int64
	SyncLogs   map[string]interface{}
	RequestID  string            `json:"RequestID,omitempty"`
	AsyncTasks []OutputAsyncTask `json:"Tasks,omitempty"`
	Meta       OutputMeta        `json:"meta"`
}

// OutputAsyncTask is the output of async tasks
type OutputAsyncTask struct {
	TaskARN  string `json:"TaskARN"`
	PublicIP string `json:"PublicIP"`
}

// OutputLogs are the set of logs
type OutputMeta struct {
	TaskDef   *ecs.RegisterTaskDefinitionInput `json:"1.taskdef,omitempty"`
	RunConfig *ecs.RunTaskInput                `json:"2.runconfig,omitempty"`
	Resources []OutputResources                `json:"3.resources,omitempty"`
	Timelines []OutputTimelines                `json:"4.timeline,omitempty"`
	ExitCodes []OutputExitCodes                `json:"5.exitcodes,omitempty"`
}

// OutputResources represent AWS resources which were used
type OutputResources struct {
	ClusterArn        string                     `json:"ClusterArn,omitempty"`
	TaskDefinitionArn string                     `json:"TaskDefinitionArn,omitempty"`
	TaskArn           string                     `json:"TaskArn,omitempty"`
	TaskRoleArn       string                     `json:"TaskRoleArn,omitempty"`
	LogGroup          string                     `json:"LogGroup,omitempty"`
	PublicIP          string                     `json:"PublicIP,omitempty"`
	Containers        []OutputContainerResources `json:"Containers,omitempty"`
}

// OutputContainerResources represent container resources
type OutputContainerResources struct {
	ContainerArn string `json:"ContainerArn,omitempty"`
	LogStream    string `json:"LogStream,omitempty"`
}

// OutputTimelines are the series of the events
type OutputTimelines struct {
	AppStartedAt              string `json:"0,omitempty"`
	AppTriedToRunFargateAt    string `json:"1,omitempty"`
	FargateCreatedAt          string `json:"2,omitempty"`
	FargatePullStartedAt      string `json:"3,omitempty"`
	FargatePullStoppedAt      string `json:"4,omitempty"`
	FargateStartedAt          string `json:"5,omitempty"`
	FargateExecutionStoppedAt string `json:"6,omitempty"`
	FargateStoppedAt          string `json:"7,omitempty"`
	AppRetrievedLogsAt        string `json:"8,omitempty"`
	AppFinishedAt             string `json:"9,omitempty"`
}

// OutputExitCodes represent applications status
type OutputExitCodes struct {
	LastStatus    string                     `json:"TaskLastStatus,omitempty"`
	HealthStatus  string                     `json:"TaskHealthStatus,omitempty"`
	StopCode      string                     `json:"TaskStopCode,omitempty"`
	StoppedReason string                     `json:"TaskStoppedReason,omitempty"`
	Containers    []OutputContainerExitCodes `json:"Containers,omitempty"`
}

// OutputContainerExitCodes represent containers status
type OutputContainerExitCodes struct {
	ExitCode     int64  `json:"ExitCode,omitempty"`
	Reason       string `json:"Reason,omitempty"`
	LastStatus   string `json:"LastStatus,omitempty"`
	HealthStatus string `json:"HealthStatus,omitempty"`
}

var regTaskID = regexp.MustCompile("task/(.*)")

func runResults(ctx context.Context, conf *RunConfig, startedAt, runTaskAt time.Time, logsAt *time.Time, logs map[string][]*cw.OutputLogEvent, taskdef *ecs.RegisterTaskDefinitionInput, runconfig *ecs.RunTaskInput, tasks []*ecs.Task) *Output {
	result := &Output{ExitCode: exitNormally}

	if aws.BoolValue(conf.Asynchronous) { // Async mode
		result.RequestID = requestID
		if len(tasks) > 0 {
			asyncTasks := []OutputAsyncTask{}
			for _, task := range tasks {
				asyncTasks = append(asyncTasks, OutputAsyncTask{
					TaskARN:  aws.StringValue(task.TaskArn),
					PublicIP: lib.RetrievePublicIP(ctx, conf.Aws.AccessKey, conf.Aws.SecretKey, conf.Aws.Region, task, conf.Common.IsDebugMode),
				})
			}
			result.AsyncTasks = asyncTasks
		}
	} else { // Sync mode
		result.SyncLogs = map[string]interface{}{}
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
			result.SyncLogs[fmt.Sprintf("container-%d", seq)] = messages
			seq++
		}
	}
	timelines := []OutputTimelines{}
	resources := []OutputResources{}
	exitcodes := []OutputExitCodes{}
	if len(tasks) > 0 {
		for _, task := range tasks {
			containers := []OutputContainerResources{}
			taskID := ""
			matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
			if len(matched) > 0 && len(matched[0]) > 1 {
				taskID = strings.Replace(matched[0][1], requestID+"/", "", -1)
			}
			for _, container := range task.Containers {
				containers = append(containers, OutputContainerResources{
					ContainerArn: aws.StringValue(container.ContainerArn),
					LogStream:    fmt.Sprintf("fargate/%s/%s", aws.StringValue(container.Name), taskID),
				})
			}
			container := taskdef.ContainerDefinitions[0]
			resource := OutputResources{
				ClusterArn:        aws.StringValue(task.ClusterArn),
				TaskDefinitionArn: aws.StringValue(task.TaskDefinitionArn),
				TaskArn:           aws.StringValue(task.TaskArn),
				TaskRoleArn:       aws.StringValue(taskdef.TaskRoleArn),
				LogGroup:          aws.StringValue(container.LogConfiguration.Options["awslogs-group"]),
				Containers:        containers,
			}
			if aws.BoolValue(conf.Common.ExtendedOutput) {
				resource.PublicIP = lib.RetrievePublicIP(ctx, conf.Aws.AccessKey, conf.Aws.SecretKey, conf.Aws.Region, task, conf.Common.IsDebugMode)
			}
			resources = append(resources, resource)

			timeline := OutputTimelines{
				AppStartedAt:              fmt.Sprintf("AppStartedAt:              %s", rfc3339(startedAt)),
				AppTriedToRunFargateAt:    fmt.Sprintf("AppTriedToRunFargateAt:    %s", rfc3339(runTaskAt)),
				FargateCreatedAt:          fmt.Sprintf("FargateCreatedAt:          %s", toStr(task.CreatedAt)),
				FargatePullStartedAt:      fmt.Sprintf("FargatePullStartedAt:      %s", toStr(task.PullStartedAt)),
				FargatePullStoppedAt:      fmt.Sprintf("FargatePullStoppedAt:      %s", toStr(task.PullStoppedAt)),
				FargateStartedAt:          fmt.Sprintf("FargateStartedAt:          %s", toStr(task.StartedAt)),
				FargateExecutionStoppedAt: fmt.Sprintf("FargateExecutionStoppedAt: %s", toStr(task.ExecutionStoppedAt)),
				FargateStoppedAt:          fmt.Sprintf("FargateStoppedAt:          %s", toStr(task.StoppedAt)),
				AppRetrievedLogsAt:        fmt.Sprintf("AppRetrievedLogsAt:        %s", rfc3339(aws.TimeValue(logsAt))),
				AppFinishedAt:             fmt.Sprintf("AppFinishedAt:             %s", rfc3339(time.Now())),
			}
			timelines = append(timelines, timeline)

			exitcode := OutputExitCodes{
				LastStatus:    aws.StringValue(task.LastStatus),
				HealthStatus:  aws.StringValue(task.HealthStatus),
				StopCode:      aws.StringValue(task.StopCode),
				StoppedReason: aws.StringValue(task.StoppedReason),
			}
			containerExitCodes := []OutputContainerExitCodes{}
			for _, container := range task.Containers {
				containerExitCodes = append(containerExitCodes, OutputContainerExitCodes{
					ExitCode:     aws.Int64Value(container.ExitCode),
					Reason:       aws.StringValue(container.Reason),
					LastStatus:   aws.StringValue(container.LastStatus),
					HealthStatus: aws.StringValue(container.HealthStatus),
				})
			}
			exitcode.Containers = containerExitCodes
			exitcodes = append(exitcodes, exitcode)
		}
		result.Meta = OutputMeta{
			TaskDef:   taskdef,
			RunConfig: runconfig,
			Resources: resources,
			Timelines: timelines,
			ExitCodes: exitcodes,
		}
	}
	return result
}

func stopResults(ctx context.Context, conf *StopConfig, logs map[string][]*cw.OutputLogEvent, tasks []*ecs.Task) *Output {
	result := &Output{ExitCode: exitNormally}
	result.SyncLogs = map[string]interface{}{}
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
		result.SyncLogs[fmt.Sprintf("container-%d", seq)] = messages
		seq++
	}
	timelines := []OutputTimelines{}
	resources := []OutputResources{}
	exitcodes := []OutputExitCodes{}
	if len(tasks) > 0 {
		for _, task := range tasks {
			containers := []OutputContainerResources{}
			taskID := ""
			matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
			if len(matched) > 0 && len(matched[0]) > 1 {
				taskID = strings.Replace(matched[0][1], requestID+"/", "", -1)
			}
			for _, container := range task.Containers {
				containers = append(containers, OutputContainerResources{
					ContainerArn: aws.StringValue(container.ContainerArn),
					LogStream:    fmt.Sprintf("fargate/%s/%s", aws.StringValue(container.Name), taskID),
				})
			}
			resource := OutputResources{
				ClusterArn:        aws.StringValue(task.ClusterArn),
				TaskDefinitionArn: aws.StringValue(task.TaskDefinitionArn),
				TaskArn:           aws.StringValue(task.TaskArn),
				Containers:        containers,
			}
			resources = append(resources, resource)

			timeline := OutputTimelines{
				FargateCreatedAt:          fmt.Sprintf("FargateCreatedAt:          %s", toStr(task.CreatedAt)),
				FargatePullStartedAt:      fmt.Sprintf("FargatePullStartedAt:      %s", toStr(task.PullStartedAt)),
				FargatePullStoppedAt:      fmt.Sprintf("FargatePullStoppedAt:      %s", toStr(task.PullStoppedAt)),
				FargateStartedAt:          fmt.Sprintf("FargateStartedAt:          %s", toStr(task.StartedAt)),
				FargateExecutionStoppedAt: fmt.Sprintf("FargateExecutionStoppedAt: %s", toStr(task.ExecutionStoppedAt)),
				FargateStoppedAt:          fmt.Sprintf("FargateStoppedAt:          %s", toStr(task.StoppedAt)),
				AppFinishedAt:             fmt.Sprintf("AppFinishedAt:             %s", rfc3339(time.Now())),
			}
			timelines = append(timelines, timeline)

			exitcode := OutputExitCodes{
				LastStatus:    aws.StringValue(task.LastStatus),
				HealthStatus:  aws.StringValue(task.HealthStatus),
				StopCode:      aws.StringValue(task.StopCode),
				StoppedReason: aws.StringValue(task.StoppedReason),
			}
			containerExitCodes := []OutputContainerExitCodes{}
			for _, container := range task.Containers {
				containerExitCodes = append(containerExitCodes, OutputContainerExitCodes{
					ExitCode:     aws.Int64Value(container.ExitCode),
					Reason:       aws.StringValue(container.Reason),
					LastStatus:   aws.StringValue(container.LastStatus),
					HealthStatus: aws.StringValue(container.HealthStatus),
				})
			}
			exitcode.Containers = containerExitCodes
			exitcodes = append(exitcodes, exitcode)
		}
		result.Meta = OutputMeta{
			Resources: resources,
			Timelines: timelines,
			ExitCodes: exitcodes,
		}
	}
	return result
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

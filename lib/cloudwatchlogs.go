package lib

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	cw "github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pottava/ecs-task-runner/log"
)

// CreateLogGroup creates a LogGroup
func CreateLogGroup(ctx context.Context, sess *session.Session, logGroup string) error {
	_, err := cw.New(sess).CreateLogGroupWithContext(ctx, &cw.CreateLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})
	return err
}

var regTaskID = regexp.MustCompile("task/(.*)")

// RetrieveLogs retrieve containers' logs
func RetrieveLogs(ctx context.Context, sess *session.Session, tasks []*ecs.Task, clusterName, logGroupName, streamPrefix string) map[string][]*cw.OutputLogEvent {
	logs := map[string][]*cw.OutputLogEvent{}
	for _, task := range tasks {
		matched := regTaskID.FindAllStringSubmatch(aws.StringValue(task.TaskArn), -1)
		if len(matched) > 0 && len(matched[0]) > 1 {
			taskID := strings.Replace(matched[0][1], clusterName+"/", "", -1)
			name := ""
			if len(task.Containers) > 0 {
				name = aws.StringValue(task.Containers[0].Name)
			}
			if out, err := cw.New(sess).GetLogEventsWithContext(ctx, &cw.GetLogEventsInput{
				LogGroupName:  aws.String(logGroupName),
				LogStreamName: aws.String(fmt.Sprintf("%s/%s/%s", streamPrefix, name, taskID)),
			}); err == nil {
				logs[aws.StringValue(task.TaskArn)] = out.Events
			}
		}
	}
	return logs
}

// DeleteLogGroup deletes a specified LogGroup
func DeleteLogGroup(sess *session.Session, logGroupName string, debug bool) {
	if _, err := cw.New(sess).DeleteLogGroup(&cw.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	}); err != nil && debug {
		log.PrintJSON(err)
	}
}

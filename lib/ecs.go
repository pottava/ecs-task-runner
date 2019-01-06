package lib

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pottava/ecs-task-runner/log"
)

// CreateClusterIfNotExist creates a cluster if there is not any clusters
func CreateClusterIfNotExist(ctx context.Context, sess *session.Session, cluster *string) error {
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

// StopTask stops a specified task
func StopTask(ctx context.Context, sess *session.Session, cluster, taskARN *string) (*ecs.Task, error) {
	task, err := ecs.New(sess).StopTaskWithContext(ctx, &ecs.StopTaskInput{
		Cluster: cluster,
		Task:    taskARN,
	})
	if err != nil {
		return nil, err
	}
	return task.Task, nil
}

// DeregisterTaskDef deregisters a task definition
func DeregisterTaskDef(sess *session.Session, taskDefinition *string, debug bool) {
	if _, err := ecs.New(sess).DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: taskDefinition,
	}); err != nil && debug {
		log.PrintJSON(err)
	}
}

// DeleteECSCluster deletes a ecs cluster
func DeleteECSCluster(sess *session.Session, clusterName string, debug bool) {
	if _, err := ecs.New(sess).DeleteCluster(&ecs.DeleteClusterInput{
		Cluster: aws.String(clusterName),
	}); err != nil && debug {
		log.PrintJSON(err)
	}
}

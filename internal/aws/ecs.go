package aws

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pottava/ecs-task-runner/internal/log"
)

// CreateClusterIfNotExist creates a cluster if there is not any clusters
func CreateClusterIfNotExist(ctx context.Context, sess *session.Session, cluster *string, spot *bool) (bool, error) {
	out, err := ecs.New(sess).DescribeClustersWithContext(ctx, &ecs.DescribeClustersInput{
		Clusters: []*string{cluster},
	})
	if err != nil {
		return false, err
	}
	if len(out.Clusters) > 0 {
		return true, nil
	}
	input := &ecs.CreateClusterInput{
		ClusterName: cluster,
	}
	if aws.BoolValue(spot) {
		input.CapacityProviders = []*string{fargateSpot}
		input.DefaultCapacityProviderStrategy = []*ecs.CapacityProviderStrategyItem{
			&ecs.CapacityProviderStrategyItem{
				CapacityProvider: fargateSpot,
				Weight:           aws.Int64(1),
				Base:             aws.Int64(0),
			},
		}
	}
	_, err = ecs.New(sess).CreateClusterWithContext(ctx, input)
	if err != nil {
		return false, err
	}
	timeout := time.After(15 * time.Second)
	for {
		select {
		case <-timeout:
			return false, errors.New("The task did not finish in 15 seconds")
		default:
			out, err = ecs.New(sess).DescribeClustersWithContext(ctx, &ecs.DescribeClustersInput{
				Clusters: []*string{cluster},
			})
			if err != nil {
				return false, err
			}
			if len(out.Clusters) > 0 &&
				aws.StringValue(out.Clusters[0].Status) == ecs.CapacityProviderStatusActive {
				return false, nil
			}
			time.Sleep(2 * time.Second)
		}
	}
}

var fargateSpot = aws.String("FARGATE_SPOT")

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

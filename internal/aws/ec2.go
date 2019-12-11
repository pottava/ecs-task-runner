package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/pottava/ecs-task-runner/conf"
	"github.com/pottava/ecs-task-runner/internal/log"
)

// RetrievePublicIP retrieves public IP address from ENI
func RetrievePublicIP(ctx context.Context, cfg *conf.AwsConfig, task *ecs.Task, debug bool) string {
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
	sess, err := Session(cfg, debug)
	if err != nil && debug {
		log.PrintJSON(err)
		return ""
	}
	input := &ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []*string{eniID},
	}
	eni, err := ec2.New(sess).DescribeNetworkInterfacesWithContext(ctx, input)
	if err != nil {
		if debug {
			log.PrintJSON(err)
		}
		return ""
	}
	if len(eni.NetworkInterfaces) == 0 {
		return ""
	}
	return aws.StringValue(eni.NetworkInterfaces[0].Association.PublicIp)
}

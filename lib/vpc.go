package lib

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// FindDefaultVPC finds the default VPC
func FindDefaultVPC(ctx context.Context, sess *session.Session) *string {
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

// FindDefaultSubnet finds the default Subnet
func FindDefaultSubnet(ctx context.Context, sess *session.Session, vpc *string) *string {
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

// FindDefaultSecurityGroup finds the default SecurityGroup
func FindDefaultSecurityGroup(ctx context.Context, sess *session.Session, vpc *string) *string {
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

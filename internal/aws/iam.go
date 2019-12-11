package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pottava/ecs-task-runner/internal/log"
)

// CreatePolicy creates a policy
func CreatePolicy(ctx context.Context, sess *session.Session, policyName, policyDoc string) (*iam.Policy, error) {
	out, err := iam.New(sess).CreatePolicyWithContext(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(policyDoc),
		Path:           aws.String("/"),
	})
	if err != nil {
		return nil, err
	}
	return out.Policy, nil
}

// AttachPolicy attaches a policy to some role
func AttachPolicy(ctx context.Context, sess *session.Session, roleName, policyArn *string) error {
	_, err := iam.New(sess).AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{
		RoleName:  roleName,
		PolicyArn: policyArn,
	})
	return err
}

// DeletePolicy deletes an IAM policy
func DeletePolicy(sess *session.Session, roleName, policyArn *string, debug bool) {
	if _, err := iam.New(sess).DetachRolePolicy(&iam.DetachRolePolicyInput{
		RoleName:  roleName,
		PolicyArn: policyArn,
	}); err != nil && debug {
		log.PrintJSON(err)
	}
	if _, err := iam.New(sess).DeletePolicy(&iam.DeletePolicyInput{
		PolicyArn: policyArn,
	}); err != nil && debug {
		log.PrintJSON(err)
	}
}

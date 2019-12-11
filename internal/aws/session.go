package aws

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pottava/ecs-task-runner/conf"
	"github.com/pottava/ecs-task-runner/internal/util"
)

// Session creaate a new AWS session
func Session(cfg *conf.AwsConfig, debug bool,
) (*session.Session, error) {
	if !util.IsEmpty(cfg.AccessKey) {
		os.Setenv("AWS_ACCESS_KEY_ID", aws.StringValue(cfg.AccessKey)) // nolint
	}
	if !util.IsEmpty(cfg.SecretKey) {
		os.Setenv("AWS_SECRET_ACCESS_KEY", aws.StringValue(cfg.SecretKey)) // nolint
	}
	if !util.IsEmpty(cfg.Profile) {
		os.Setenv("AWS_PROFILE", aws.StringValue(cfg.Profile)) // nolint
	}
	awsConfig := &aws.Config{
		Region: cfg.Region,
		Credentials: credentials.NewChainCredentials([]credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{
				// Use default values
				// - Filename: $HOME/.aws/credentials
				// - Profile: $AWS_PROFILE
			},
			&ec2rolecreds.EC2RoleProvider{
				Client: ec2metadata.New(session.Must(session.NewSession(&aws.Config{
					Region: cfg.Region,
				}))),
			},
		}),
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}
	if !util.IsEmpty(cfg.AssumeRole) {
		return assume(awsConfig, cfg, sess, cfg.AssumeRole)
	}
	return sess, nil
}

var sessionName = aws.String("ecs-task-runner-session")

func assume(awsConfig *aws.Config, cfg *conf.AwsConfig, sess *session.Session, roleARN *string,
) (*session.Session, error) {
	input := &sts.AssumeRoleInput{
		RoleArn:         roleARN,
		RoleSessionName: sessionName,
		DurationSeconds: aws.Int64(600),
	}
	if !util.IsEmpty(cfg.MfaSerialNumber) && !util.IsEmpty(cfg.MfaToken) {
		input.SerialNumber = cfg.MfaSerialNumber
		input.TokenCode = cfg.MfaToken
	}
	assumed, err := sts.New(sess).AssumeRole(input)
	if err != nil {
		return nil, err
	}
	awsConfig.Credentials = credentials.NewChainCredentials([]credentials.Provider{
		&credentials.StaticProvider{Value: credentials.Value{
			AccessKeyID:     aws.StringValue(assumed.Credentials.AccessKeyId),
			SecretAccessKey: aws.StringValue(assumed.Credentials.SecretAccessKey),
			SessionToken:    aws.StringValue(assumed.Credentials.SessionToken),
		}},
	})
	return session.NewSession(awsConfig)
}

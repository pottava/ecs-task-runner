package lib

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Session creaate a new AWS session
func Session(awsAccessKey, awsSecretKey, awsRegion, endpoint *string) (*session.Session, error) {
	level := aws.LogLevelType(aws.LogOff)
	if os.Getenv("DEBUG") == "1" {
		level = aws.LogLevelType(aws.LogDebug)
	}
	if awsAccessKey != nil {
		os.Setenv("AWS_ACCESS_KEY_ID", aws.StringValue(awsAccessKey))
	}
	if awsSecretKey != nil {
		os.Setenv("AWS_SECRET_ACCESS_KEY", aws.StringValue(awsSecretKey))
	}
	cfg := &aws.Config{
		Region:   awsRegion,
		LogLevel: &level,
	}
	if endpoint != nil {
		cfg.Endpoint = endpoint
		cfg.S3ForcePathStyle = aws.Bool(true)
		cfg.DisableSSL = aws.Bool(true)
	}
	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

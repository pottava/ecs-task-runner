package aws

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pottava/ecs-task-runner/conf"
	"github.com/stretchr/testify/assert"
)

func TestSession(t *testing.T) {
	sess, err := Session(&conf.AwsConfig{}, false)
	assert.NotNil(t, sess)
	assert.Nil(t, err)
}

func TestSessionWithAllOptions(t *testing.T) {
	os.Setenv("DEBUG", "1")
	sess, err := Session(
		&conf.AwsConfig{
			Region:          aws.String("ap-northeast-1"),
			AccessKey:       aws.String("AKIAIOSFODNN7EXAMPLE"),
			SecretKey:       aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			Profile:         aws.String("default"),
			AssumeRole:      aws.String("arn:aws:iam::123456789012:role/role"),
			MfaSerialNumber: nil,
			MfaToken:        nil,
		},
		false,
	)
	assert.Nil(t, sess)
	assert.NotNil(t, err)
}

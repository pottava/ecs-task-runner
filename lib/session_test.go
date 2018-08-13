package lib

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestSession(t *testing.T) {
	sess, err := Session(nil, nil, nil, nil)
	assert.NotNil(t, sess)
	assert.Nil(t, err)
}

func TestSessionWithAllOptions(t *testing.T) {
	os.Setenv("DEBUG", "1")
	sess, err := Session(
		aws.String("AKIAIOSFODNN7EXAMPLE"),
		aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
		aws.String("ap-northeast-1"),
		aws.String("http://localhost"),
	)
	assert.NotNil(t, sess)
	assert.Nil(t, err)
}

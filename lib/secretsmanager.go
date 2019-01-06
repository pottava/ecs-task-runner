package lib

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	sm "github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pottava/ecs-task-runner/log"
)

// CreateSecret creates a secret
func CreateSecret(ctx context.Context, sess *session.Session, name, kmsCustomKeyID, secret *string) (*string, error) {
	out, err := sm.New(sess).CreateSecretWithContext(ctx, &sm.CreateSecretInput{
		Name:         name,
		KmsKeyId:     kmsCustomKeyID,
		SecretString: secret,
	})
	if err != nil {
		return nil, err
	}
	return out.ARN, nil
}

// DeleteSecret deletes a specified secret
func DeleteSecret(sess *session.Session, secretID *string, force, debug bool) {
	if secretID == nil {
		return
	}
	if _, err := sm.New(sess).DeleteSecret(&sm.DeleteSecretInput{
		SecretId:                   secretID,
		ForceDeleteWithoutRecovery: aws.Bool(force),
	}); err != nil && debug {
		log.PrintJSON(err)
	}
}

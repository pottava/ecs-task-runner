package util

import (
	"github.com/aws/aws-sdk-go/aws"
)

// IsEmpty returns the value has a valid value or not
func IsEmpty(candidate *string) bool {
	return candidate == nil || aws.StringValue(candidate) == ""
}

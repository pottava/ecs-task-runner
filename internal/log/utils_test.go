package log

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	Logger = log.New(buf, "", 0)

	PrintJSON(1)

	assert.Equal(t, "1\n", buf.String())
}

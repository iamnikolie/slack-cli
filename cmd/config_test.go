package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveInitToken(t *testing.T) {
	t.Setenv("SLK_TOKEN", "") // hermetic: ignore any ambient token

	// flag wins
	tok, err := resolveInitToken("xoxp-flag", strings.NewReader("xoxp-stdin\n"))
	assert.NoError(t, err)
	assert.Equal(t, "xoxp-flag", tok)

	// stdin fallback, trimmed
	tok, err = resolveInitToken("", strings.NewReader("  xoxp-stdin  \n"))
	assert.NoError(t, err)
	assert.Equal(t, "xoxp-stdin", tok)

	// empty everywhere errors
	_, err = resolveInitToken("", strings.NewReader("\n"))
	assert.Error(t, err)
}

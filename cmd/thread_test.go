package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveThreadTargetPermalink(t *testing.T) {
	ch, ts, err := resolveThreadTarget(context.Background(),
		[]string{"https://acme.slack.com/archives/C0123/p1700000000123456"})
	require.NoError(t, err)
	assert.Equal(t, "C0123", ch)
	assert.Equal(t, "1700000000.123456", ts)
}

func TestResolveThreadTargetBareTSNeedsChannelArg(t *testing.T) {
	_, _, err := resolveThreadTarget(context.Background(), []string{"1700000000.123456"})
	assert.Error(t, err) // one arg + bare ts → ambiguous channel
}

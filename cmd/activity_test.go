package cmd

import (
	"encoding/json"
	"testing"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityUsesLatestMessageAndSampleCounts(t *testing.T) {
	d := &cache.Directory{Channels: []cache.Channel{{ID: "D1", Name: "mako", IsIM: true}, {ID: "C1", Name: "general"}}}
	rows, err := summarizeActivity([]json.RawMessage{
		json.RawMessage(`{"channel":{"id":"C1"},"text":"old","ts":"1700000000.000100"}`),
		json.RawMessage(`{"channel":{"id":"D1"},"text":"hello\nworld","ts":"1700000300.000100","permalink":"https://example.com/dm"}`),
		json.RawMessage(`{"channel":{"id":"C1"},"text":"new","ts":"1700000200.000100"}`),
	}, d)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "@mako", rows[0].Channel)
	assert.Equal(t, "im", rows[0].Type)
	assert.Equal(t, "hello world", rows[0].Preview)
	assert.Equal(t, "https://example.com/dm", rows[0].Permalink)
	assert.Equal(t, 1, rows[0].SampledMessages)
	assert.Equal(t, "new", rows[1].Preview)
	assert.Equal(t, 2, rows[1].SampledMessages)
	empty, err := summarizeActivity(nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

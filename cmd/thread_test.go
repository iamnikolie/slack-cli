package cmd

import (
	"context"
	"net/url"
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

func TestParseThreadRefPrefersParentOfReplyPermalink(t *testing.T) {
	ch, ts, ok := parseThreadRef("https://acme.slack.com/archives/C0123456/p1700000100000200?thread_ts=1700000000.000100&cid=C0123456")
	require.True(t, ok)
	assert.Equal(t, "C0123456", ch)
	assert.Equal(t, "1700000000.000100", ts)

	_, ts, ok = parseThreadRef("1700000100.000200")
	require.True(t, ok)
	assert.Equal(t, "1700000100.000200", ts) // bare ts is taken as given
}

func TestFetchRepliesKeepsLatestAcrossPages(t *testing.T) {
	var limits []string
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.replies": func(f map[string]string) string {
			assert.NotEmpty(t, f["latest"], "oldest alone would page from the oldest end")
			limits = append(limits, f["limit"])
			// Slack pages a thread newest first and repeats the parent each page.
			if f["cursor"] == "" {
				return `{"ok":true,"has_more":true,"messages":[{"ts":"1.000000"},{"ts":"7.000000"},{"ts":"8.000000"}],
				  "response_metadata":{"next_cursor":"p2"}}`
			}
			return `{"ok":true,"has_more":true,"messages":[{"ts":"1.000000"},{"ts":"5.000000"},{"ts":"6.000000"}],
			  "response_metadata":{"next_cursor":"p3"}}`
		},
	})
	raw, more, err := fetchReplies(context.Background(), "C0123456", "1.000000", 3, url.Values{"oldest": {"0.5"}})
	require.NoError(t, err)
	assert.True(t, more)
	assert.Equal(t, []string{"3", "1"}, limits)
	assert.JSONEq(t, `[{"ts":"1.000000"},{"ts":"6.000000"},{"ts":"7.000000"},{"ts":"8.000000"}]`, string(raw))
}

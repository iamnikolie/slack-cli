package cmd

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoDegradesOnMissingScopes(t *testing.T) {
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.info": func(f map[string]string) string {
			assert.Equal(t, "true", f["include_num_members"])
			return `{"ok":true,"channel":{"id":"C0123456","name":"eng","created":1700000000,"creator":"U1",
			  "num_members":7,"topic":{"value":"ship it"},"purpose":{"value":"engineering"}}}`
		},
		"bookmarks.list": func(map[string]string) string {
			return `{"ok":true,"bookmarks":[{"title":"Runbook","link":"https://wiki/runbook"}]}`
		},
		"pins.list": func(map[string]string) string {
			return `{"ok":false,"error":"missing_scope","needed":"pins:read"}`
		},
	})
	info, err := fetchChannelInfo(context.Background(), "C0123456")
	require.NoError(t, err)
	assert.Equal(t, "ship it", info.Topic)
	assert.Equal(t, 7, info.Members)
	assert.Equal(t, []channelBookmark{{Title: "Runbook", Link: "https://wiki/runbook"}}, info.Bookmarks)
	assert.JSONEq(t, `[]`, string(info.Pins))
	assert.Equal(t, []string{"pins: needs the pins:read scope"}, info.Unavailable)

	oldStdout, oldNoLinks := os.Stdout, noLinks
	t.Cleanup(func() { os.Stdout, noLinks, dir = oldStdout, oldNoLinks, nil })
	dir, noLinks = &cache.Directory{Users: []cache.User{{ID: "U1", Name: "mako"}}}, true
	f, err := os.CreateTemp(t.TempDir(), "out")
	require.NoError(t, err)
	os.Stdout = f
	require.NoError(t, printChannelInfo(context.Background(), info))
	out, _ := os.ReadFile(f.Name())
	assert.Contains(t, string(out), "members: 7 · created 2023-11-1")
	assert.Contains(t, string(out), "by @mako")
	assert.Contains(t, string(out), "- Runbook — https://wiki/runbook")
	assert.Contains(t, string(out), "(pins: needs the pins:read scope)")
}

func TestDescribeUser(t *testing.T) {
	oldCLI := cli
	t.Cleanup(func() { cli = oldCLI })
	cli = nil // presence is best-effort; no client means none
	u := map[string]any{"id": "U1", "tz_offset": float64(7200), "profile": map[string]any{
		"title": "CTO", "status_emoji": ":palm_tree:", "status_text": "vacation", "status_expiration": float64(0),
	}}
	describeUser(context.Background(), u, time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-10-02 22:00 Fri", u["local_time"])
	assert.Equal(t, "CTO", u["title"])
	assert.Equal(t, ":palm_tree: vacation", u["status"])
	assert.NotContains(t, u, "presence")
}

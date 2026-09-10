package cmd

import (
	"testing"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscriptDateSourceAndReplyLink(t *testing.T) {
	oldLocal, oldProfile := time.Local, profile
	t.Cleanup(func() { time.Local, profile = oldLocal, oldProfile })
	time.Local, profile = time.FixedZone("test", 3600), "work"
	d := &cache.Directory{Channels: []cache.Channel{{ID: "C123456", Name: "general"}}}
	out, err := renderTranscript([]byte(`[{"user":"U1","text":"reply","ts":"1700000100.000200","thread_ts":"1700000000.000100"}]`), d, "C123456", "https://acme.slack.com/", "1700000000.000100")
	require.NoError(t, err)
	assert.Contains(t, out, "# work — #general (C123456) — thread 1700000000.000100")
	assert.Contains(t, out, "[2023-11-14 23:15:00 +01:00]")
	assert.Contains(t, out, "https://acme.slack.com/archives/C123456/p1700000100000200?thread_ts=1700000000.000100&cid=C123456")
	assert.Empty(t, messagePermalink("", "C123456", "1700000000.000100", ""))
	assert.Empty(t, messagePermalink("https://acme.slack.com", "C123456", "invalid", ""))
}

func TestExpandMentions(t *testing.T) {
	d := &cache.Directory{Users: []cache.User{{ID: "U999", Name: "mako"}}}
	got := expandMentions("hi <@U999> and <@U000>", d)
	assert.Equal(t, "hi @mako and @U000", got)
}

func TestRenderTranscript(t *testing.T) {
	d := &cache.Directory{Users: []cache.User{{ID: "U1", Name: "mako"}}}
	msgs := []byte(`[
	  {"user":"U1","text":"hello <@U1>","ts":"1700000000.000100","reply_count":2},
	  {"user":"U1","text":"world","ts":"1700000100.000200"}
	]`)
	out, err := renderTranscript(msgs, d, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, out, "@mako")
	assert.Contains(t, out, "hello @mako")
	assert.Contains(t, out, "ts=1700000000.000100")
	assert.Contains(t, out, "↳ 2 replies")
}

func TestTranscriptAuthorFallbacks(t *testing.T) {
	byID := map[string]string{"U1": "mako"}
	assert.Equal(t, "mako", transcriptAuthor(transcriptMsg{User: "U1"}, byID))
	assert.Equal(t, "U9", transcriptAuthor(transcriptMsg{User: "U9"}, byID)) // unknown id → raw
	assert.Equal(t, "deploybot", transcriptAuthor(transcriptMsg{Username: "deploybot"}, byID))
	assert.Equal(t, "bot:B123", transcriptAuthor(transcriptMsg{BotID: "B123"}, byID))
	assert.Equal(t, "channel_join", transcriptAuthor(transcriptMsg{Subtype: "channel_join"}, byID))
	assert.Equal(t, "system", transcriptAuthor(transcriptMsg{}, byID))
}

func TestRenderTranscriptNoEmptyAuthor(t *testing.T) {
	msgs := []byte(`[{"subtype":"bot_message","username":"CI","text":"build green","ts":"1700000000.000100"}]`)
	out, err := renderTranscript(msgs, nil, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, out, "@CI: build green")
	assert.NotContains(t, out, "@: ") // never a bare @
}

func TestTranscriptShowsDownloadableFileID(t *testing.T) {
	out, err := renderTranscript([]byte(`[{"text":"screenshot","ts":"1700000000.000100","files":[{"id":"F01234567","name":"screen.png","mimetype":"image/png","size":1234}]}]`), nil, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, out, "file F01234567: screen.png (image/png, 1234 bytes)")
}

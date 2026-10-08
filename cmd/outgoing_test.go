package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliverSendsMarkdownUnlessMrkdwn(t *testing.T) {
	var got []map[string]string
	fakeSlack(t, map[string]func(map[string]string) string{
		"chat.postMessage": func(f map[string]string) string {
			got = append(got, f)
			return `{"ok":true,"channel":"C0123456","ts":"1700000000.000100"}`
		},
	})
	_, err := deliver(context.Background(), outgoing{Channel: "C0123456", Thread: "1690000000.000100", Text: "**hi**"}, &msgFlags{})
	require.NoError(t, err)
	_, err = deliver(context.Background(), outgoing{Channel: "C0123456", Text: "*hi*"}, &msgFlags{mrkdwn: true})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "**hi**", got[0]["markdown_text"])
	assert.Equal(t, "1690000000.000100", got[0]["thread_ts"])
	assert.NotContains(t, got[0], "text")
	assert.Equal(t, "*hi*", got[1]["text"])
	assert.NotContains(t, got[1], "markdown_text")

	_, err = deliver(context.Background(), outgoing{Channel: "C0123456", Text: strings.Repeat("x", markdownLimit+1)}, &msgFlags{})
	assert.ErrorContains(t, err, "--mrkdwn")
}

func TestDeliverSchedulesAndUpdates(t *testing.T) {
	var methods []string
	var postAt string
	fakeSlack(t, map[string]func(map[string]string) string{
		"chat.scheduleMessage": func(f map[string]string) string {
			methods, postAt = append(methods, "schedule"), f["post_at"]
			return `{"ok":true,"channel":"C0123456","scheduled_message_id":"Q1","post_at":1900000000}`
		},
		"chat.update": func(f map[string]string) string {
			methods = append(methods, "update:"+f["ts"])
			return `{"ok":true,"channel":"C0123456","ts":"1700000000.000100"}`
		},
	})
	at := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	_, err := deliver(context.Background(), outgoing{Channel: "C0123456", At: at, Text: "later"}, &msgFlags{})
	require.NoError(t, err)
	_, err = deliver(context.Background(), outgoing{Channel: "C0123456", TS: "1700000000.000100", Text: "fixed"}, &msgFlags{})
	require.NoError(t, err)
	assert.Equal(t, []string{"schedule", "update:1700000000.000100"}, methods)
	assert.Equal(t, at.Unix(), mustInt(t, postAt))

	_, err = deliver(context.Background(), outgoing{Channel: "C0123456", At: time.Now().Add(-time.Minute), Text: "x"}, &msgFlags{})
	assert.ErrorContains(t, err, "in the past")
}

func TestDryRunSendsNothing(t *testing.T) {
	fakeSlack(t, map[string]func(map[string]string) string{}) // any call fails the test
	oldDry := dryRun
	t.Cleanup(func() { dryRun, dir = oldDry, nil })
	dryRun = true
	dir = &cache.Directory{Channels: []cache.Channel{{ID: "C0123456", Name: "eng"}}}
	raw, err := deliver(context.Background(), outgoing{Channel: "C0123456", Thread: "1690000000.000100", Text: "hi"}, &msgFlags{})
	require.NoError(t, err)
	var plan map[string]any
	require.NoError(t, json.Unmarshal(raw, &plan))
	assert.Equal(t, true, plan["dry_run"])
	assert.Equal(t, "chat.postMessage", plan["method"])
	assert.Equal(t, "#eng", plan["channel_name"])
	assert.Equal(t, "markdown_text", plan["format"])
	assert.Equal(t, "1690000000.000100", plan["thread_ts"])
}

func TestChannelRefResolvesUserToDM(t *testing.T) {
	t.Cleanup(func() { dir = nil })
	dir = &cache.Directory{
		Users:    []cache.User{{ID: "U0000001", Name: "mako"}, {ID: "U0000002", Name: "ira"}},
		Channels: []cache.Channel{{ID: "D0000001", IsIM: true, User: "U0000001", Name: "mako"}},
	}
	var opened []string
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.open": func(f map[string]string) string {
			opened = append(opened, f["users"])
			return `{"ok":true,"channel":{"id":"D0000002"}}`
		},
	})
	id, err := channelRef(context.Background(), "@mako")
	require.NoError(t, err)
	assert.Equal(t, "D0000001", id) // existing DM, no API call
	id, err = channelRef(context.Background(), "U0000002")
	require.NoError(t, err)
	assert.Equal(t, "D0000002", id)
	assert.Equal(t, []string{"U0000002"}, opened)
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, json.Unmarshal([]byte(s), &n))
	return n
}

func TestReactTargetsExactMessage(t *testing.T) {
	var got []map[string]string
	fakeSlack(t, map[string]func(map[string]string) string{
		"reactions.add":    func(f map[string]string) string { got = append(got, f); return `{"ok":true}` },
		"reactions.remove": func(f map[string]string) string { got = append(got, f); return `{"ok":true}` },
	})
	// A reply's permalink: the reaction goes on the reply, not the parent.
	link := "https://acme.slack.com/archives/C0123456/p1700000100000200?thread_ts=1700000000.000100&cid=C0123456"
	require.NoError(t, runReact(context.Background(), []string{link, ":+1:"}, false))
	require.NoError(t, runReact(context.Background(), []string{"C0123456", "1700000000.000100", "eyes"}, true))
	require.Len(t, got, 2)
	assert.Equal(t, map[string]string{"channel": "C0123456", "timestamp": "1700000100.000200", "name": "+1"}, got[0])
	assert.Equal(t, "eyes", got[1]["name"])
	assert.Error(t, runReact(context.Background(), []string{"1700000000.000100", "eyes"}, false))
}

func TestLinkMentions(t *testing.T) {
	d := &cache.Directory{Users: []cache.User{{ID: "U1", Name: "andriir"}, {ID: "U2", Name: "oleksii.k"}}}
	cases := map[string]struct{ in, want string }{
		"start and middle":               {"@andriir please look, @Oleksii.K too", "<@U1> please look, <@U2> too"},
		"trailing punctuation":           {"thanks @andriir. And (@andriir)", "thanks <@U1>. And (<@U1>)"},
		"existing markup":                {"<@U1> and <https://x.io|@andriir>", "<@U1> and <https://x.io|@andriir>"},
		"email and url":                  {"a@andriir.com https://h/@andriir", "a@andriir.com https://h/@andriir"},
		"code kept as typed":             {"run `slk dm @andriir` or\n```\n@andriir\n```\n@andriir", "run `slk dm @andriir` or\n```\n@andriir\n```\n<@U1>"},
		"broadcast left as typed":        {"@here look", "@here look"},
		"link label kept as typed":       {"<https://x.io|ping @andriir> @andriir", "<https://x.io|ping @andriir> <@U1>"},
		"tilde fence kept as typed":      {"~~~\n@andriir\n~~~", "~~~\n@andriir\n~~~"},
		"italic and trailing underscore": {"_@andriir_ and @andriir_", "_<@U1>_ and <@U1>_"},
	}
	for name, c := range cases {
		got, _ := linkMentions(c.in, d)
		assert.Equal(t, c.want, got, name)
	}
	_, unknown := linkMentions("@andriir @nobody and @nobody @here", d)
	assert.Equal(t, []string{"@nobody"}, unknown)
}

func TestDeliverLinksMentions(t *testing.T) {
	var got map[string]string
	fakeSlack(t, map[string]func(map[string]string) string{
		"chat.postMessage": func(f map[string]string) string {
			got = f
			return `{"ok":true,"channel":"C0123456","ts":"1700000000.000100"}`
		},
	})
	oldStderr := stderr
	var warn strings.Builder
	t.Cleanup(func() { dir, stderr = nil, oldStderr })
	dir = &cache.Directory{Users: []cache.User{{ID: "U1", Name: "andriir"}}}
	stderr = &warn
	_, err := deliver(context.Background(), outgoing{Channel: "C0123456", Text: "@andriir and @ghost please review"}, &msgFlags{})
	require.NoError(t, err)
	assert.Equal(t, "<@U1> and @ghost please review", got["markdown_text"])
	assert.Contains(t, warn.String(), "@ghost")
	_, err = deliver(context.Background(), outgoing{Channel: "C0123456", Text: "@andriir hi"}, &msgFlags{mrkdwn: true})
	require.NoError(t, err)
	assert.Equal(t, "<@U1> hi", got["text"])
}

func TestDeliverSendsWhenDirectoryUnavailable(t *testing.T) {
	var got map[string]string
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.list": func(map[string]string) string { return `{"ok":false,"error":"missing_scope"}` },
		"chat.postMessage": func(f map[string]string) string {
			got = f
			return `{"ok":true,"channel":"C0123456","ts":"1700000000.000100"}`
		},
	})
	t.Setenv("SLK_HOME", t.TempDir())
	oldStderr := stderr
	var warn strings.Builder
	t.Cleanup(func() { dir, stderr = nil, oldStderr })
	dir, stderr = nil, &warn
	_, err := deliver(context.Background(), outgoing{Channel: "C0123456", Text: "mail me at a@b.com"}, &msgFlags{})
	require.NoError(t, err)
	assert.Equal(t, "mail me at a@b.com", got["markdown_text"])
	assert.Contains(t, warn.String(), "missing_scope")
}

package cmd

import (
	"strings"
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

func TestExpandMentionsDecodesSlackMarkup(t *testing.T) {
	d := &cache.Directory{
		Users:    []cache.User{{ID: "U999", Name: "mako"}},
		Channels: []cache.Channel{{ID: "C111", Name: "general"}},
	}
	in := "<@U999> see <#C111> and <#C222|design>, ping <!subteam^S1|@devs> <!here> " +
		"<https://a.io/x?a=1&amp;b=2|the doc> <https://b.io> <mailto:x@y.io|x@y.io> " +
		"<!date^1700000000^{date}|Nov 14> a &lt; b &amp;&amp; c &gt; d"
	assert.Equal(t, "@mako see #general and #design, ping @devs @here "+
		"the doc (https://a.io/x?a=1&b=2) https://b.io x@y.io "+
		"Nov 14 a < b && c > d", expandMentions(in, d))
	assert.Equal(t, "@U000 #C000 @S2", expandMentions("<@U000> <#C000> <!subteam^S2>", nil))
}

func TestTranscriptRichMessage(t *testing.T) {
	oldNoLinks, oldMax := noLinks, maxChars
	t.Cleanup(func() { noLinks, maxChars = oldNoLinks, oldMax })
	msg := []byte(`[{"user":"U1","text":"look <https://loom.com/x>","ts":"1700000000.000100","edited":{"user":"U1","ts":"1700000001.000000"},
	  "reactions":[{"name":"+1","count":3,"users":[]},{"name":"eyes","count":1}],
	  "attachments":[{"service_name":"Loom","title":"Meeting","from_url":"https://loom.com/x","text":"long summary"},
	                 {"title":"Deploy failed","title_link":"https://ci/1","text":"step &lt;b&gt; &amp; more"}]},
	 {"bot_id":"B1","text":"","ts":"1700000100.000100","blocks":[{"type":"section","text":{"type":"mrkdwn","text":"build *green*"}},
	   {"type":"context","elements":[{"type":"mrkdwn","text":"by ci"}]}]}]`)

	out, err := renderTranscript(msg, nil, "C123456", "https://acme.slack.com", "")
	require.NoError(t, err)
	assert.Contains(t, out, "ts=1700000000.000100 @U1 (edited): look https://loom.com/x\n")
	assert.Contains(t, out, "  ▸ Loom: Meeting\n")
	assert.Contains(t, out, "  ▸ Deploy failed (https://ci/1) — step <b> & more\n")
	assert.Contains(t, out, "  reactions: :+1:×3 :eyes:×1\n")
	assert.Contains(t, out, "@bot:B1: build *green*\nby ci\n")
	assert.Contains(t, out, "https://acme.slack.com/archives/C123456/p1700000000000100")

	noLinks, maxChars = true, 4
	out, err = renderTranscript(msg, nil, "C123456", "https://acme.slack.com", "")
	require.NoError(t, err)
	assert.NotContains(t, out, "https://acme.slack.com")
	assert.Contains(t, out, ": look…(+19 chars)\n")
}

func TestExpandMentionsCollapsesSlackShortenedLinks(t *testing.T) {
	const mr = "https://gitlab.example.com/erp/app/-/merge_requests/832"
	for in, want := range map[string]string{
		"<" + mr + "|gitlab.example.com/erp/app/…/832>":                mr, // Slack elided the middle
		"<" + mr + "|gitlab.example.com/erp/app/-/merge_requests/832>": mr, // scheme dropped only
		"<" + mr + "|MR !832>":                        "MR !832 (" + mr + ")", // a real label stays
		"<" + mr + "|gitlab.example.com/other/…/999>": "gitlab.example.com/other/…/999 (" + mr + ")",
	} {
		assert.Equal(t, want, expandMentions(in, nil), in)
	}
}

func TestTranscriptLiftsBotAttachmentAndMarksExternalFiles(t *testing.T) {
	oldNoLinks := noLinks
	t.Cleanup(func() { noLinks = oldNoLinks })
	noLinks = true
	out, err := renderTranscript([]byte(`[{"username":"Alertmanager","text":"","ts":"1700000000.000100",
	  "attachments":[{"title":"FIRING Heartbeat"},{"title":"second"}],
	  "files":[{"id":"F1","name":"Plan","mimetype":"application/vnd.google-apps.document","size":0,"is_external":true}]}]`), nil, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, out, "@Alertmanager: FIRING Heartbeat\n")
	assert.Equal(t, 1, strings.Count(out, "FIRING Heartbeat"))
	assert.Contains(t, out, "  ▸ second\n")
	assert.Contains(t, out, "file F1: Plan (application/vnd.google-apps.document, external)")
}

func TestReadableTextExpandsAndCaps(t *testing.T) {
	oldMax := maxChars
	t.Cleanup(func() { maxChars = oldMax })
	maxChars = 8
	d := &cache.Directory{Users: []cache.User{{ID: "U1", Name: "ann"}}}
	msgs, err := decodeArray(readableText([]byte(`[{"ts":"1","text":"<@U1> hello world"}]`), d))
	require.NoError(t, err)
	assert.Equal(t, "@ann hel…(+8 chars)", msgs[0]["text"])
}

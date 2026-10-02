package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/iamnikolie/slack-cli/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFocusWindow(t *testing.T) {
	raw := []byte(`[{"ts":"1.000001"},{"ts":"2.000002"},{"ts":"3.000003"},{"ts":"4.000004"},{"ts":"5.000005"}]`)
	out, err := focusWindow(raw, "4.000004", 1, true)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"ts":"1.000001"},{"ts":"3.000003"},{"ts":"4.000004","slk_focus":true},{"ts":"5.000005"}]`, string(out))

	out, err = focusWindow(raw, "1.000001", 0, false)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"ts":"1.000001","slk_focus":true}]`, string(out))

	_, err = focusWindow(raw, "9.000009", 1, false)
	assert.Error(t, err)
}

func TestResolveMessageTargetKeepsReplyAndParent(t *testing.T) {
	ch, ts, thread, err := resolveMessageTarget(context.Background(),
		[]string{"https://acme.slack.com/archives/C0123456/p1700000100000200?thread_ts=1700000000.000100&cid=C0123456"})
	require.NoError(t, err)
	assert.Equal(t, []string{"C0123456", "1700000100.000200", "1700000000.000100"}, []string{ch, ts, thread})

	_, _, _, err = resolveMessageTarget(context.Background(), []string{"1700000100.000200"})
	assert.Error(t, err) // bare ts without a channel
}

// fakeSlack serves canned responses per API method.
func fakeSlack(t *testing.T, responses map[string]func(form map[string]string) string) {
	t.Helper()
	oldCLI := cli
	t.Cleanup(func() { cli = oldCLI; identityCache = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form := map[string]string{}
		for k := range r.Form {
			form[k] = r.Form.Get(k)
		}
		h, ok := responses[r.URL.Path[1:]]
		if !ok {
			t.Errorf("unexpected call %s", r.URL.Path)
			fmt.Fprint(w, `{"ok":false,"error":"unknown_method"}`)
			return
		}
		fmt.Fprint(w, h(form))
	}))
	t.Cleanup(srv.Close)
	cli = client.New("test", srv.URL)
}

func TestUnreadInSplitsReadMarkerAndFollowedThreads(t *testing.T) {
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.info": func(map[string]string) string {
			return `{"ok":true,"channel":{"last_read":"1700000200.000000"}}`
		},
		"conversations.history": func(f map[string]string) string {
			assert.Equal(t, "1690000000.000000", f["oldest"])
			// newest first, as Slack returns it
			return `{"ok":true,"messages":[
			  {"ts":"1700000400.000000","user":"UME","text":"mine, skipped"},
			  {"ts":"1700000300.000000","user":"U2","text":"new"},
			  {"ts":"1700000150.000000","user":"U2","text":"read, no thread"},
			  {"ts":"1700000100.000000","user":"U2","text":"old parent","reply_count":3,"subscribed":true,
			   "last_read":"1700000110.000000","latest_reply":"1700000500.000000"},
			  {"ts":"1700000050.000000","user":"U2","text":"unfollowed","reply_count":2,"subscribed":false,
			   "last_read":"0","latest_reply":"1700000600.000000"}]}`
		},
		"conversations.replies": func(f map[string]string) string {
			assert.Equal(t, "1700000100.000000", f["ts"])
			assert.Equal(t, "1700000110.000000", f["oldest"])
			return `{"ok":true,"messages":[
			  {"ts":"1700000100.000000","user":"U2","text":"old parent"},
			  {"ts":"1700000500.000000","user":"U3","text":"new reply"},
			  {"ts":"1700000510.000000","user":"UME","text":"my reply"}]}`
		},
	})
	msgs, err := unreadIn(context.Background(), "C0123456", "UME", "1690000000.000000")
	require.NoError(t, err)
	b, _ := json.Marshal(msgs)
	var got []struct {
		Text    string `json:"text"`
		Replies []struct {
			Text string `json:"text"`
		} `json:"replies"`
	}
	require.NoError(t, json.Unmarshal(b, &got))
	require.Len(t, got, 2)
	assert.Equal(t, "old parent", got[0].Text)
	require.Len(t, got[0].Replies, 1)
	assert.Equal(t, "new reply", got[0].Replies[0].Text)
	assert.Equal(t, "new", got[1].Text)
	assert.Nil(t, got[1].Replies)
}

func TestTailMovesCursorUnlessPeek(t *testing.T) {
	t.Setenv("SLK_HOME", t.TempDir())
	oldProfile, oldSince, oldPeek, oldReplies, oldLimit := profile, digestSince, tailPeek, digestReplies, digestLimit
	t.Cleanup(func() {
		profile, digestSince, tailPeek, digestReplies, digestLimit = oldProfile, oldSince, oldPeek, oldReplies, oldLimit
	})
	profile, digestSince, digestReplies, digestLimit, jsonOutput = "test", "1d", false, 100, true
	t.Cleanup(func() { jsonOutput = false; dir = nil })
	dir = &cache.Directory{}

	var oldest []string
	fakeSlack(t, map[string]func(map[string]string) string{
		"conversations.history": func(f map[string]string) string {
			oldest = append(oldest, f["oldest"])
			return `{"ok":true,"messages":[{"ts":"1700000300.000000","text":"b"},{"ts":"1700000200.000000","text":"a"}]}`
		},
	})
	require.NoError(t, cache.SaveCursors(cache.Cursors{"C0123456": "1700000100.000000"}, "test"))

	tailCmd.SetContext(context.Background())
	tailPeek = true
	require.NoError(t, tailCmd.RunE(tailCmd, []string{"C0123456"}))
	c, err := cache.LoadCursors("test")
	require.NoError(t, err)
	assert.Equal(t, "1700000100.000000", c["C0123456"]) // peek leaves it

	tailPeek = false
	require.NoError(t, tailCmd.RunE(tailCmd, []string{"C0123456"}))
	c, err = cache.LoadCursors("test")
	require.NoError(t, err)
	assert.Equal(t, "1700000300.000000", c["C0123456"])
	assert.Equal(t, []string{"1700000100.000000", "1700000100.000000"}, oldest)
}

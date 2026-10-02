package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamnikolie/slack-cli/internal/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimeBound(t *testing.T) {
	// already a ts → passthrough
	ts, err := parseTimeBound("1700000000.000100")
	require.NoError(t, err)
	assert.Equal(t, "1700000000.000100", ts)

	// epoch seconds → passthrough
	ts, err = parseTimeBound("1700000000")
	require.NoError(t, err)
	assert.Equal(t, "1700000000", ts)

	// YYYY-MM-DD → epoch seconds string
	ts, err = parseTimeBound("2023-11-14")
	require.NoError(t, err)
	assert.Equal(t, "1699920000", ts) // 2023-11-14T00:00:00Z

	_, err = parseTimeBound("not-a-date")
	assert.Error(t, err)
}

func TestExpandRepliesAttachesThreadsUnderParents(t *testing.T) {
	oldCLI := cli
	t.Cleanup(func() { cli = oldCLI })
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/conversations.replies", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "C0123456", r.Form.Get("channel"))
		assert.Equal(t, "3", r.Form.Get("limit")) // replies-limit 2 + parent
		calls = append(calls, r.Form.Get("ts"))
		fmt.Fprint(w, `{"ok":true,"messages":[
		  {"ts":"1700000000.000100","text":"parent","reply_count":5},
		  {"ts":"1700000100.000200","thread_ts":"1700000000.000100","text":"r1"},
		  {"ts":"1700000200.000300","thread_ts":"1700000000.000100","text":"r2"}]}`)
	}))
	defer srv.Close()
	cli = client.New("test", srv.URL)

	raw := []byte(`[
	  {"ts":"1700000000.000100","text":"parent","reply_count":5},
	  {"ts":"1700000300.000400","thread_ts":"1700000000.000100","reply_count":5,"subtype":"thread_broadcast","text":"broadcast"},
	  {"ts":"1700000500.000500","text":"plain"}]`)
	out, err := expandReplies(context.Background(), raw, "C0123456", 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"1700000000.000100"}, calls) // one call, broadcast/plain skipped

	transcript, err := renderTranscript(out, nil, "C0123456", "", "")
	require.NoError(t, err)
	assert.Contains(t, transcript, "    ↳ [")
	assert.Contains(t, transcript, "r1\tts=1700000100.000200")
	assert.Contains(t, transcript, "↳ +3 more replies: slk thread C0123456 1700000000.000100")
	assert.Equal(t, 1, strings.Count(transcript, "parent\t")) // parent not repeated among replies

	flat, err := decodeArray(flattenReplies(out))
	require.NoError(t, err)
	var texts []string
	for _, m := range flat {
		texts = append(texts, m["text"].(string))
		assert.NotContains(t, m, "replies")
	}
	assert.Equal(t, []string{"parent", "r1", "r2", "broadcast", "plain"}, texts)
}

func TestExpandRepliesKeepsGoingWhenAThreadFails(t *testing.T) {
	oldCLI, oldStderr := cli, stderr
	t.Cleanup(func() { cli, stderr = oldCLI, oldStderr })
	var errOut strings.Builder
	stderr = &errOut
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":false,"error":"thread_not_found"}`)
	}))
	defer srv.Close()
	cli = client.New("test", srv.URL)

	out, err := expandReplies(context.Background(), []byte(`[{"ts":"1700000000.000100","reply_count":2}]`), "C0123456", 50)
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "thread 1700000000.000100: replies unavailable")
	transcript, err := renderTranscript(out, nil, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, transcript, "↳ 2 replies") // unexpanded thread still advertised
}

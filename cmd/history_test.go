package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iamnikolie/slack-cli/internal/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimeBound(t *testing.T) {
	oldLocal := time.Local
	t.Cleanup(func() { time.Local = oldLocal })
	time.Local = time.UTC
	// already a ts → passthrough
	ts, err := parseTimeBound("1700000000.000100")
	require.NoError(t, err)
	assert.Equal(t, "1700000000.000100", ts)

	// epoch seconds → passthrough
	ts, err = parseTimeBound("1700000000")
	require.NoError(t, err)
	assert.Equal(t, "1700000000", ts)

	// YYYY-MM-DD → local midnight as epoch seconds
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
		assert.Equal(t, "2", r.Form.Get("limit")) // replies-limit; Slack adds the parent
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
	out, err := expandReplies(context.Background(), raw, "C0123456", 2, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"1700000000.000100"}, calls) // one call, broadcast/plain skipped

	transcript, err := renderTranscript(out, nil, "C0123456", "", "")
	require.NoError(t, err)
	assert.Contains(t, transcript, "    ↳ [")
	assert.Contains(t, transcript, "ts=1700000100.000200 @system: r1")
	assert.Contains(t, transcript, "↳ +3 earlier replies: slk thread C0123456 1700000000.000100")
	assert.Equal(t, 1, strings.Count(transcript, ": parent\n")) // parent not repeated among replies

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

	out, err := expandReplies(context.Background(), []byte(`[{"ts":"1700000000.000100","reply_count":2}]`), "C0123456", 50, "")
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "thread 1700000000.000100: replies unavailable")
	transcript, err := renderTranscript(out, nil, "", "", "")
	require.NoError(t, err)
	assert.Contains(t, transcript, "↳ 2 replies") // unexpanded thread still advertised
}

func TestParsePastAndFuture(t *testing.T) {
	loc := time.FixedZone("test", 2*3600)
	now := time.Date(2026, 10, 2, 15, 30, 0, 0, loc)
	for in, want := range map[string]time.Time{
		"2h":               now.Add(-2 * time.Hour),
		"3d":               now.AddDate(0, 0, -3),
		"1w":               now.AddDate(0, 0, -7),
		"today":            time.Date(2026, 10, 2, 0, 0, 0, 0, loc),
		"yesterday":        time.Date(2026, 10, 1, 0, 0, 0, 0, loc),
		"2026-09-30 08:15": time.Date(2026, 9, 30, 8, 15, 0, 0, loc),
	} {
		got, err := parsePast(in, now)
		require.NoError(t, err, in)
		assert.True(t, want.Equal(got), "%s: %v != %v", in, got, want)
	}
	_, err := parsePast("+2h", now)
	assert.Error(t, err) // a future offset is not a past bound

	for in, want := range map[string]time.Time{
		"+2h":              now.Add(2 * time.Hour),
		"in 30m":           now.Add(30 * time.Minute),
		"16:00":            time.Date(2026, 10, 2, 16, 0, 0, 0, loc),
		"09:00":            time.Date(2026, 10, 3, 9, 0, 0, 0, loc), // passed today → tomorrow
		"tomorrow 10:30":   time.Date(2026, 10, 3, 10, 30, 0, 0, loc),
		"tomorrow":         time.Date(2026, 10, 3, 9, 0, 0, 0, loc),
		"2026-10-05 12:00": time.Date(2026, 10, 5, 12, 0, 0, 0, loc),
	} {
		got, err := parseFuture(in, now)
		require.NoError(t, err, in)
		assert.True(t, want.Equal(got), "%s: %v != %v", in, got, want)
	}
}

func TestSortByTS(t *testing.T) {
	in := []byte(`[{"ts":"1700000002.000000"},{"ts":"1700000003.000000"},{"ts":"1700000001.000000"}]`)
	assert.JSONEq(t, `[{"ts":"1700000001.000000"},{"ts":"1700000002.000000"},{"ts":"1700000003.000000"}]`, string(sortByTS(in, false)))
	assert.JSONEq(t, `[{"ts":"1700000003.000000"},{"ts":"1700000002.000000"},{"ts":"1700000001.000000"}]`, string(sortByTS(in, true)))
}

func TestPinLatest(t *testing.T) {
	q := pinLatest(url.Values{"oldest": {"1700000000"}})
	assert.NotEmpty(t, q.Get("latest"), "oldest alone makes Slack page from the oldest end")
	q = pinLatest(url.Values{"oldest": {"1"}, "latest": {"2"}})
	assert.Equal(t, "2", q.Get("latest"))
	assert.Empty(t, pinLatest(url.Values{}).Get("latest"))
}

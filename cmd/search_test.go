package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/iamnikolie/slack-cli/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSearchQuery(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	q := func(base string, f searchFilters) string {
		got, err := buildSearchQuery(base, f, now)
		require.NoError(t, err)
		return got
	}
	assert.Equal(t, "deploy failed", q("deploy failed", searchFilters{}))
	assert.Equal(t, "deploy in:ops from:mako", q("deploy", searchFilters{In: "#ops", From: "@mako"}))
	assert.Equal(t, "to:me after:2026-09-28 before:2026-10-02 has:link has:pin is:thread",
		q("", searchFilters{To: "me", Since: "3d", Until: "today", Has: []string{"link,pin"}, Threads: true}))
	assert.Equal(t, "on:2026-09-15", q("", searchFilters{On: "2026-09-15"}))

	_, err := buildSearchQuery("", searchFilters{}, now)
	assert.Error(t, err)
}

func TestSearchCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name, response  string
		total, returned int
		more, next      bool
	}{
		{"empty", `{"ok":true,"messages":{"matches":[],"total":0,"paging":{"pages":0}}}`, 0, 0, false, false},
		{"next page", `{"ok":true,"messages":{"matches":[{"ts":"1"}],"total":3,"paging":{"pages":3}}}`, 3, 1, true, true},
		{"complete", `{"ok":true,"messages":{"matches":[{"ts":"1"}],"pagination":{"total_count":1,"page_count":1}}}`, 1, 1, false, false},
		{"empty partial", `{"ok":true,"messages":{"matches":[],"total":3,"paging":{"pages":3}}}`, 3, 0, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.response) }))
			defer srv.Close()
			oldCLI := cli
			t.Cleanup(func() { cli = oldCLI })
			cli = client.New("test", srv.URL)
			result, err := fetchSearch(context.Background(), "q", 1, "desc")
			require.NoError(t, err)
			assert.Equal(t, tc.total, result.SearchMeta.Total)
			assert.Equal(t, tc.returned, result.SearchMeta.Returned)
			assert.Equal(t, tc.more, result.SearchMeta.HasMore)
			assert.Equal(t, tc.next, result.SearchMeta.HasNextPage)
			assert.Equal(t, 1, result.SearchMeta.PagesFetched)
		})
	}
}

func TestSearchJSONProjectionKeepsMetadata(t *testing.T) {
	oldFields, oldStdout := fieldsFlag, os.Stdout
	t.Cleanup(func() { fieldsFlag, os.Stdout = oldFields, oldStdout })
	fieldsFlag = []string{"ts", "user", "text", "permalink", "thread_ts"}
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer f.Close()
	os.Stdout = f
	err = emitSearchEnvelope(searchResult{
		Matches:    []json.RawMessage{json.RawMessage(`{"ts":"1","text":"\u0442\u0430\u043a","blocks":[{}],"files":[{"preview":"huge"}],"thread_ts":"0"}`)},
		SearchMeta: searchMeta{Total: 42, Returned: 1, HasMore: true, HasNextPage: true},
	})
	require.NoError(t, err)
	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	var got searchResult
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, 42, got.SearchMeta.Total)
	assert.True(t, got.SearchMeta.HasMore)
	assert.JSONEq(t, `{"ts":"1","text":"так","thread_ts":"0"}`, string(got.Matches[0]))
	assert.NotContains(t, string(data), "blocks")
	assert.NotContains(t, string(data), "preview")
	assert.Contains(t, string(data), "так")
}

func TestSearchPaginationKeepsPageSizeAndOrder(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search.messages", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "100", r.Form.Get("count"))
		assert.Equal(t, "asc", r.Form.Get("sort_dir"))
		assert.Equal(t, "timestamp", r.Form.Get("sort"))
		page, _ := strconv.Atoi(r.Form.Get("page"))
		requests++
		assert.Equal(t, requests, page)
		matches := make([]map[string]string, 100)
		for i := range matches {
			matches[i] = map[string]string{"ts": fmt.Sprintf("%010d.000100", (page-1)*100+i)}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true, "messages": map[string]any{"matches": matches, "total": 200, "pagination": map[string]int{"page_count": 2}}}))
	}))
	defer srv.Close()
	oldCLI := cli
	t.Cleanup(func() { cli = oldCLI })
	cli = client.New("test", srv.URL)
	result, err := fetchSearch(context.Background(), "hello", 150, "asc")
	require.NoError(t, err)
	matches := result.Matches
	require.Len(t, matches, 150)
	assert.Equal(t, 200, result.SearchMeta.Total)
	assert.True(t, result.SearchMeta.HasMore)
	assert.False(t, result.SearchMeta.HasNextPage)
	assert.Equal(t, 50, result.SearchMeta.OmittedFromLastPage)
	assert.Equal(t, 150, result.SearchMeta.Returned)
	assert.Equal(t, 2, requests)
	assert.JSONEq(t, `{"ts":"0000000100.000100"}`, string(matches[100]))
	assert.JSONEq(t, `{"ts":"0000000149.000100"}`, string(matches[149]))
}

func TestBuildSearchQueryRejectsEmptyWindow(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	_, err := buildSearchQuery("x", searchFilters{Since: "2026-10-03", Until: "2026-10-02"}, now)
	assert.Error(t, err)
	_, err = buildSearchQuery("x", searchFilters{Since: "2026-10-02", Until: "2026-10-03"}, now)
	assert.NoError(t, err)
}

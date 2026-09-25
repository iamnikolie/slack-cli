package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamnikolie/slack-cli/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadFilesSharesToThread(t *testing.T) {
	oldCLI, oldTransport := cli, http.DefaultTransport
	t.Cleanup(func() { cli, http.DefaultTransport = oldCLI, oldTransport })
	var completeForm map[string]string
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		switch r.URL.Path {
		case "/files.getUploadURLExternal":
			n++
			assert.NotEmpty(t, r.Form.Get("filename"))
			assert.Equal(t, "4", r.Form.Get("length"))
			fmt.Fprintf(w, `{"ok":true,"upload_url":"https://files.slack.com/upload/v1/x%d","file_id":"F0000000%d"}`, n, n)
		case "/files.completeUploadExternal":
			completeForm = map[string]string{}
			for k := range r.Form {
				completeForm[k] = r.Form.Get(k)
			}
			fmt.Fprint(w, `{"ok":true,"files":[]}`)
		default:
			t.Fatalf("unexpected method %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	var uploaded []string
	http.DefaultTransport = fileTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "files.slack.com" {
			assert.Empty(t, r.Header.Get("Authorization"), "token must not reach upload_url")
			b, _ := io.ReadAll(r.Body)
			uploaded = append(uploaded, string(b))
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("OK")), Request: r}, nil
		}
		return oldTransport.RoundTrip(r)
	})
	cli = client.New("test", srv.URL)

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.png")
	require.NoError(t, os.WriteFile(a, []byte("aaaa"), 0600))
	require.NoError(t, os.WriteFile(b, []byte("bbbb"), 0600))

	out, err := uploadFiles(context.Background(), []string{a, b}, uploadOpts{Channel: "C01234567", Thread: "1700000000.000100", Comment: "see attached"})
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "F00000001", out[0].ID)
	assert.Equal(t, "b.png", out[1].Name)
	assert.Equal(t, []string{"aaaa", "bbbb"}, uploaded)
	assert.Equal(t, "C01234567", completeForm["channel_id"])
	assert.Equal(t, "1700000000.000100", completeForm["thread_ts"])
	assert.Equal(t, "see attached", completeForm["initial_comment"])
	var files []map[string]string
	require.NoError(t, json.Unmarshal([]byte(completeForm["files"]), &files))
	assert.Equal(t, []map[string]string{{"id": "F00000001", "title": "a.txt"}, {"id": "F00000002", "title": "b.png"}}, files)
}

func TestUploadFilesValidatesBeforeUploading(t *testing.T) {
	oldCLI := cli
	t.Cleanup(func() { cli = oldCLI })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no API call expected, got %s", r.URL.Path)
	}))
	defer srv.Close()
	cli = client.New("test", srv.URL)
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.txt")
	empty := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(ok, []byte("x"), 0600))
	require.NoError(t, os.WriteFile(empty, nil, 0600))

	for _, tc := range []struct {
		paths []string
		o     uploadOpts
		want  string
	}{
		{[]string{ok, filepath.Join(dir, "missing")}, uploadOpts{}, "no such file"},
		{[]string{empty}, uploadOpts{}, "empty file"},
		{[]string{dir}, uploadOpts{}, "not a regular file"},
		{[]string{ok, ok}, uploadOpts{Name: "x"}, "--name"},
		{[]string{ok}, uploadOpts{Thread: "1.2"}, "--thread requires --channel"},
		{[]string{ok}, uploadOpts{Comment: "hi"}, "--comment requires --channel"},
	} {
		_, err := uploadFiles(context.Background(), tc.paths, tc.o)
		require.ErrorContains(t, err, tc.want)
	}
}

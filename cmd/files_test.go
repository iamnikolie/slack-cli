package cmd

import (
	"context"
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

func TestFileRefs(t *testing.T) {
	for _, ref := range []string{"F01234567", "https://acme.slack.com/files/U01234567/F01234567/image.png", "https://app.slack.com/client/T01234567/C01234567/file/F01234567", "https://files.slack.com/files-pri/T01234567-F01234567/download/image.png", "https://slack-files.com/T01234567-F01234567-secret"} {
		id, err := fileRef(ref)
		require.NoError(t, err)
		assert.Equal(t, "F01234567", id)
	}
	for _, ref := range []string{"https://evil.test/files/F01234567", "https://acme.slack.com/archives/C01234567/p1700000000000100", "bad"} {
		_, err := fileRef(ref)
		require.Error(t, err)
	}
	assert.Equal(t, "F01234567-secret.png", safeFileName("F01234567", "../../secret.png"))
	assert.Equal(t, "F01234567-secret.png", safeFileName("F01234567", `..\secret.png`))
}

type fileTransport func(*http.Request) (*http.Response, error)

func (f fileTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadFileSavesPrivatelyAndCleansFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		size     int
		existing bool
		wantErr  string
	}{
		{"success", 200, 4, false, ""}, {"forbidden", 403, 4, false, "HTTP 403"}, {"short", 200, 100, false, "incomplete"}, {"existing", 200, 4, true, "already exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldCLI, oldTransport := cli, http.DefaultTransport
			t.Cleanup(func() { cli, http.DefaultTransport = oldCLI, oldTransport })
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/files.info", r.URL.Path)
				require.NoError(t, r.ParseForm())
				assert.Equal(t, "F01234567", r.Form.Get("file"))
				fmt.Fprintf(w, `{"ok":true,"file":{"id":"F01234567","name":"../../image.png","mimetype":"image/png","size":%d,"url_private":"https://files.slack.com/private"}}`, tc.size)
			}))
			defer srv.Close()
			http.DefaultTransport = fileTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "files.slack.com" {
					assert.Equal(t, "Bearer test", r.Header.Get("Authorization"))
					return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data")), Request: r}, nil
				}
				return oldTransport.RoundTrip(r)
			})
			cli = client.New("test", srv.URL)
			dest := t.TempDir()
			path := filepath.Join(dest, "F01234567-image.png")
			if tc.existing {
				require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
			}
			result, err := downloadFile(context.Background(), "F01234567", dest)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.existing {
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, "original", string(data))
				} else {
					_, err := os.Stat(path)
					require.True(t, os.IsNotExist(err))
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, path, result.Path)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "data", string(data))
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
			}
			leftovers, err := filepath.Glob(filepath.Join(dest, ".slk-download-*"))
			require.NoError(t, err)
			assert.Empty(t, leftovers)
		})
	}
}

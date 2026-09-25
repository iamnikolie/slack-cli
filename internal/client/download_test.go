package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func downloadResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestDownloadAuthenticatesAndStripsTokenAcrossRedirects(t *testing.T) {
	c := New("secret", "https://slack.com/api")
	requests := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		assert.Equal(t, "GET", r.Method)
		if requests == 1 {
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
			resp := downloadResponse(r, 302, "")
			resp.Header.Set("Location", "https://cdn.files.slack.com/signed") // even a subdomain must not receive credentials
			return resp, nil
		}
		assert.Empty(t, r.Header.Get("Authorization"))
		return downloadResponse(r, 200, "PNG bytes"), nil
	})
	var buf bytes.Buffer
	n, err := c.Download(context.Background(), "https://files.slack.com/private", "image/png", &buf)
	require.NoError(t, err)
	assert.Equal(t, int64(9), n)
	assert.Equal(t, "PNG bytes", buf.String())
	assert.Equal(t, 2, requests)
}

func TestDownloadRejectsUnsafeURLsBeforeSendingToken(t *testing.T) {
	c := New("secret", "https://slack.com/api")
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { t.Fatal("unexpected request"); return nil, nil })
	for _, u := range []string{"http://files.slack.com/x", "https://files.slack.com.evil.test/x", "https://evil.test/x", "https://user@files.slack.com/x", "https://files.slack.com:8443/x"} {
		_, err := c.Download(context.Background(), u, "image/png", io.Discard)
		require.Error(t, err)
	}
}

func TestDownloadAccessErrorsAndHTMLAreNotWritten(t *testing.T) {
	for _, tc := range []struct {
		status     int
		mime, want string
	}{{403, "text/plain", "files:read"}, {401, "text/plain", "access denied"}, {404, "text/plain", "unavailable"}, {200, "text/html", "sign-in page"}} {
		t.Run(tc.want, func(t *testing.T) {
			c := New("secret", "https://slack.com/api")
			c.Profile = "work"
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				resp := downloadResponse(r, tc.status, "error or login page")
				resp.Header.Set("Content-Type", tc.mime)
				return resp, nil
			})
			var buf bytes.Buffer
			_, err := c.Download(context.Background(), "https://files.slack.com/private", "image/png", &buf)
			require.ErrorContains(t, err, tc.want)
			assert.Empty(t, buf.String())
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestDownloadRetriesRateLimitAndRejectsDowngrade(t *testing.T) {
	c := New("secret", "https://slack.com/api")
	calls := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			resp := downloadResponse(r, 429, "")
			resp.Header.Set("Retry-After", "0")
			return resp, nil
		}
		resp := downloadResponse(r, 302, "")
		resp.Header.Set("Location", "http://files.slack.com/insecure")
		return resp, nil
	})
	_, err := c.Download(context.Background(), "https://files.slack.com/private", "image/png", io.Discard)
	require.ErrorContains(t, err, "unsafe")
	assert.Equal(t, 2, calls)
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestDownloadPropagatesWriteFailure(t *testing.T) {
	c := New("secret", "https://slack.com/api")
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return downloadResponse(r, 200, "data"), nil })
	_, err := c.Download(context.Background(), "https://files.slack.com/private", "image/png", failedWriter{})
	require.ErrorContains(t, err, "disk full")
}

func TestUploadRejectsUntrustedHostAndOmitsToken(t *testing.T) {
	c := New("secret", "https://slack.com/api")
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "POST", r.Method)
		assert.Empty(t, r.Header.Get("Authorization"))
		return downloadResponse(r, 200, "OK"), nil
	})
	require.NoError(t, c.Upload(context.Background(), "https://files.slack.com/upload/v1/x", strings.NewReader("abc"), 3))
	require.Error(t, c.Upload(context.Background(), "https://evil.test/upload", strings.NewReader("abc"), 3))
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return downloadResponse(r, 500, "boom"), nil
	})
	require.ErrorContains(t, c.Upload(context.Background(), "https://files.slack.com/upload/v1/x", strings.NewReader("abc"), 3), "HTTP 500")
}

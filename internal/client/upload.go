package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Upload POSTs raw file bytes to an upload_url returned by
// files.getUploadURLExternal. The URL is pre-signed, so the OAuth token is
// never sent, and only Slack file hosts are accepted.
func (c *Client) Upload(ctx context.Context, rawURL string, src io.Reader, size int64) error {
	u, err := url.Parse(rawURL)
	if err != nil || !trustedFileURL(u) {
		return fmt.Errorf("unsupported upload URL: expected HTTPS on a Slack file host")
	}
	httpClient := *c.http
	httpClient.Timeout = 5 * time.Minute
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("refusing file upload redirect")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), src)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "→ POST upload_url (%d bytes)\n", size)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("file upload: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file upload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

package client

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Download streams a private file URL returned by files.info. The OAuth token
// must never reach an external file provider or a cross-origin redirect.
func (c *Client) Download(ctx context.Context, rawURL, expectedMIME string, dst io.Writer) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !trustedFileURL(u) {
		return 0, fmt.Errorf("unsupported private file URL: expected HTTPS on a Slack file host")
	}
	httpClient := *c.http
	httpClient.Timeout = 5 * time.Minute
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many file download redirects")
		}
		if req.URL.Scheme != "https" || req.URL.User != nil {
			return fmt.Errorf("refusing unsafe file download redirect")
		}
		// net/http forwards credentials to subdomains by default; explicitly
		// remove them unless the entire redirect chain stays on the original host.
		for _, previous := range append(via, req) {
			if !strings.EqualFold(previous.URL.Host, u.Host) {
				req.Header.Del("Authorization")
				break
			}
		}
		return nil
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := httpClient.Do(req)
		if err != nil {
			return 0, fmt.Errorf("file download: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt == 3 {
				return 0, fmt.Errorf("file download rate limited after 3 retries")
			}
			wait := c.RetryWait * time.Duration(1<<attempt)
			if wait <= 0 {
				wait = time.Second
			}
			if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				wait = time.Duration(seconds) * time.Second
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return 0, fmt.Errorf("file download HTTP %d: access denied despite sending the profile token; verify files:read, file/channel access, workspace/profile, and Slack Connect or workspace restrictions. %s", resp.StatusCode, ScopeHelp("files:read", c.Profile))
			case http.StatusNotFound:
				return 0, fmt.Errorf("file download HTTP 404: file is unavailable, deleted, or inaccessible to this profile")
			default:
				return 0, fmt.Errorf("file download HTTP %d", resp.StatusCode)
			}
		}
		actual, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		expected, _, _ := mime.ParseMediaType(expectedMIME)
		if actual == "text/html" && expected != "text/html" {
			resp.Body.Close()
			return 0, fmt.Errorf("file download returned HTML instead of %s (possible Slack sign-in page); check token and file access", expectedMIME)
		}
		n, err := io.Copy(dst, resp.Body)
		closeErr := resp.Body.Close()
		if err != nil {
			return n, fmt.Errorf("file download interrupted: %w", err)
		}
		return n, closeErr
	}
	return 0, fmt.Errorf("file download retries exhausted")
}

func trustedFileURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "files.slack.com" || host == "files.slack-edge.com"
}

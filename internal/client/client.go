package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// APIError is a Slack logical error (HTTP 200 with {"ok":false,"error":code}).
type APIError struct {
	Code     string
	Needed   string
	Provided string
	Method   string
	Profile  string
}

func (e *APIError) Error() string {
	if e.Code == "missing_scope" {
		needed := e.Needed
		if needed == "" {
			needed = "not reported by Slack (check the method's required scopes)"
		}
		msg := fmt.Sprintf("slack error: missing_scope (%s): needed=%s", e.Method, needed)
		if e.Provided != "" {
			msg += "; provided=" + e.Provided
		}
		return msg + "\n" + ScopeHelp(e.Needed, e.Profile)
	}
	if hint, ok := errorHints[e.Code]; ok {
		return fmt.Sprintf("slack error: %s — %s", e.Code, hint)
	}
	return "slack error: " + e.Code
}

// ScopeHelp explains reauthorization without ever including the token itself.
func ScopeHelp(scopes, profile string) string {
	if scopes == "" {
		scopes = "the required scopes"
	}
	if profile == "" {
		profile = "<name>"
	}
	return fmt.Sprintf("Open https://api.slack.com/apps → your app → OAuth & Permissions → User Token Scopes; add %s, then Reinstall to Workspace and approve (workspace admin approval may be required). Save the resulting User OAuth Token using stdin: slk --config '%s' config init. If SLK_TOKEN is set, update or unset it: it overrides the saved token.", scopes, strings.ReplaceAll(profile, "'", "'\\''"))
}

// errorHints maps common Slack error codes to a human/agent recovery hint.
var errorHints = map[string]string{
	"not_in_channel":      "join the channel (or the token lacks the *_history scope)",
	"channel_not_found":   "unknown channel; run 'slk sync' then 'slk channels --filter ...'",
	"missing_scope":       "the token is missing a required OAuth scope (see README setup)",
	"invalid_auth":        "token is invalid; re-issue it and run 'slk config init'",
	"token_revoked":       "token was revoked; re-issue it and run 'slk config init'",
	"not_authed":          "no token; run 'slk config init' or set SLK_TOKEN",
	"ratelimited":         "rate limited; retry shortly",
	"cant_update_message": "you can only edit your own messages",
	"cant_delete_message": "you can only delete your own messages",
	"message_not_found":   "no message at that ts in this channel",
	"file_not_found":      "file ID is unknown or inaccessible to this token; check the file link, workspace/profile, and channel access",
	"file_deleted":        "the file has been deleted",
	"not_visible":         "this token cannot view the file; check workspace/profile, channel membership, and Slack Connect access",
	"access_denied":       "access denied; check workspace/profile, channel membership, Slack Connect, and workspace policies",
}

// Client is the Slack Web API HTTP client.
type Client struct {
	token   string
	baseURL string // e.g. https://slack.com/api
	http    *http.Client
	Verbose bool
	Profile string
	// RetryWait is the initial 429 backoff (when Retry-After is absent/0).
	RetryWait time.Duration
}

func New(token, baseURL string) *Client {
	return &Client{
		token:     token,
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: 30 * time.Second},
		RetryWait: time.Second,
	}
}

// Call POSTs method with form params, retries 429, and returns the raw body
// after verifying {"ok":true}. A false ok yields an *APIError.
func (c *Client) Call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	const maxRetries = 3
	if params == nil {
		params = url.Values{}
	}
	endpoint := c.baseURL + "/" + method
	wait := c.RetryWait
	if wait <= 0 {
		wait = time.Second
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		if c.Verbose {
			fmt.Fprintf(os.Stderr, "→ POST %s %s\n", method, params.Encode())
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		b, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Slack response: %w", readErr)
		}

		if c.Verbose {
			fmt.Fprintf(os.Stderr, "← HTTP %d\n%s\n", resp.StatusCode, string(b))
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == maxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			d := wait
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(ra); e == nil {
					d = time.Duration(secs) * time.Second
				}
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
			wait *= 2
			continue
		}
		var head struct {
			OK       bool   `json:"ok"`
			Error    string `json:"error"`
			Needed   string `json:"needed"`
			Provided string `json:"provided"`
		}
		decodeErr := json.Unmarshal(b, &head)
		if decodeErr == nil && head.Error != "" && !head.OK {
			if head.Needed == "" {
				head.Needed = resp.Header.Get("X-Accepted-OAuth-Scopes")
			}
			if head.Provided == "" {
				head.Provided = resp.Header.Get("X-OAuth-Scopes")
			}
			if head.Needed == "" && method == "files.info" {
				head.Needed = "files:read"
			}
			return nil, &APIError{Code: head.Error, Needed: head.Needed, Provided: head.Provided, Method: method, Profile: c.Profile}
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("client.Call: decode: %w", decodeErr)
		}
		if !head.OK {
			return nil, fmt.Errorf("Slack response did not confirm success")
		}
		return b, nil
	}
	return nil, fmt.Errorf("unreachable")
}

// Paginate repeatedly calls method following response_metadata.next_cursor,
// accumulating the top-level array under arrayKey up to limit items. Returns
// the combined array (capped at limit) and whether the limit was reached.
func (c *Client) Paginate(ctx context.Context, method string, params url.Values, arrayKey string, limit int) (json.RawMessage, bool, error) {
	if limit <= 0 {
		limit = 1
	}
	base := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			base.Add(k, v)
		}
	}

	var all []json.RawMessage
	cursor := ""
	for len(all) < limit {
		q := url.Values{}
		for k, vs := range base {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		page := min(limit-len(all), 200)
		q.Set("limit", strconv.Itoa(page))
		if cursor != "" {
			q.Set("cursor", cursor)
		}

		b, err := c.Call(ctx, method, q)
		if err != nil {
			return nil, false, err
		}
		var env struct {
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		_ = json.Unmarshal(b, &env)

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, false, fmt.Errorf("client.Paginate: decode: %w", err)
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw[arrayKey], &items); err != nil {
			return nil, false, fmt.Errorf("client.Paginate: %q not an array: %w", arrayKey, err)
		}
		all = append(all, items...)

		cursor = env.Meta.NextCursor
		if cursor == "" {
			break
		}
	}

	hitLimit := len(all) >= limit
	if len(all) > limit {
		all = all[:limit]
	}
	out, err := json.Marshal(all)
	if err != nil {
		return nil, false, fmt.Errorf("client.Paginate: marshal: %w", err)
	}
	return out, hitLimit, nil
}

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOKFalseMapsToError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	c := New("xoxp-x", srv.URL)
	_, err := c.Call(context.Background(), "conversations.info", nil)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "channel_not_found", apiErr.Code)
}

func TestCallOKTruePassesBodyThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer xoxp-x", r.Header.Get("Authorization"))
		w.Write([]byte(`{"ok":true,"user":"U1","team":"T1"}`))
	}))
	defer srv.Close()

	c := New("xoxp-x", srv.URL)
	body, err := c.Call(context.Background(), "auth.test", nil)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	assert.Equal(t, "U1", m["user"])
}

func TestCallRetriesOn429(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("xoxp-x", srv.URL)
	c.RetryWait = time.Millisecond
	_, err := c.Call(context.Background(), "auth.test", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, hits)
}

func TestPaginateFollowsCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("cursor") == "" {
			w.Write([]byte(`{"ok":true,"channels":[{"id":"C1"}],"response_metadata":{"next_cursor":"page2"}}`))
			return
		}
		w.Write([]byte(`{"ok":true,"channels":[{"id":"C2"}],"response_metadata":{"next_cursor":""}}`))
	}))
	defer srv.Close()

	c := New("xoxp-x", srv.URL)
	out, hit, err := c.Paginate(context.Background(), "conversations.list", nil, "channels", 100)
	require.NoError(t, err)
	assert.False(t, hit)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(out, &items))
	assert.Len(t, items, 2)
	assert.Equal(t, "C1", items[0]["id"])
	assert.Equal(t, "C2", items[1]["id"])
}

func TestErrorHintIncluded(t *testing.T) {
	e := &APIError{Code: "invalid_auth"}
	assert.Contains(t, e.Error(), "invalid_auth")
	assert.Contains(t, e.Error(), "config init")
}

func TestMissingScopeIncludesExactScopeAndReauthorization(t *testing.T) {
	for _, status := range []int{200, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"ok":false,"error":"missing_scope","needed":"files:read","provided":"search:read,users:read"}`))
			}))
			defer srv.Close()
			c := New("secret", srv.URL)
			c.Profile = "work"
			_, err := c.Call(context.Background(), "files.info", nil)
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, "files:read", apiErr.Needed)
			for _, text := range []string{"needed=files:read", "provided=search:read,users:read", "User Token Scopes", "Reinstall to Workspace", "slk --config 'work' config init", "SLK_TOKEN"} {
				assert.Contains(t, err.Error(), text)
			}
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestMissingScopeFallsBackToHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accepted-OAuth-Scopes", "search:read")
		w.Header().Set("X-OAuth-Scopes", "users:read")
		w.Write([]byte(`{"ok":false,"error":"missing_scope"}`))
	}))
	defer srv.Close()
	_, err := New("secret", srv.URL).Call(context.Background(), "search.messages", nil)
	require.ErrorContains(t, err, "needed=search:read; provided=users:read")
}

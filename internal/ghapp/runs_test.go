// internal/ghapp/runs_test.go
package ghapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListWorkflowRuns(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runs":
			require.Equal(t, "queued", r.URL.Query().Get("status"))
			require.Equal(t, "10", r.URL.Query().Get("per_page"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workflow_runs": []map[string]any{
					{"id": 1, "name": "smoke", "status": "queued",
					 "run_started_at": "2026-05-13T12:00:00Z",
					 "head_branch": "main"},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	got, _, err := c.ListWorkflowRuns(t.Context(), "acme/foo", "queued", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "smoke", got[0].Name)
	require.Equal(t, "queued", got[0].Status)
}

func TestListWorkflowRuns_ETag304(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runs":
			if r.Header.Get("If-None-Match") == `"abc123"` {
				w.Header().Set("ETag", `"abc123"`)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"abc123"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": []any{}})
		}
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	_, etag, err := c.ListWorkflowRuns(t.Context(), "acme/foo", "queued", "")
	require.NoError(t, err)
	require.Equal(t, `"abc123"`, etag)

	got, etag2, err := c.ListWorkflowRuns(t.Context(), "acme/foo", "queued", etag)
	require.NoError(t, err)
	require.Equal(t, `"abc123"`, etag2)
	require.Nil(t, got, "304 should return nil slice")
}

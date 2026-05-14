// internal/ghapp/runners_test.go
package ghapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListAndDeleteRunner(t *testing.T) {
	keyPath, _ := writeKey(t)
	var deletedID int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runners":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runners": []map[string]any{
					{"id": 1, "name": "bobsled-h1-1"},
					{"id": 2, "name": "manual-runner"},
				},
			})
		case "/repos/acme/foo/actions/runners/42":
			require.Equal(t, http.MethodDelete, r.Method)
			deletedID = 42
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}

	got, err := c.ListRepoRunners(t.Context(), "acme/foo")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "bobsled-h1-1", got[0].Name)

	require.NoError(t, c.DeleteRepoRunner(t.Context(), "acme/foo", 42))
	require.Equal(t, int64(42), deletedID)
}

func TestListRepoRunnersETag(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runners":
			if r.Header.Get("If-None-Match") == `"xyz"` {
				w.Header().Set("ETag", `"xyz"`)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"xyz"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"runners": []map[string]any{}})
		}
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}

	_, etag, err := c.ListRepoRunnersETag(t.Context(), "acme/foo", "")
	require.NoError(t, err)
	require.Equal(t, `"xyz"`, etag)

	got, etag2, err := c.ListRepoRunnersETag(t.Context(), "acme/foo", etag)
	require.NoError(t, err)
	require.Equal(t, `"xyz"`, etag2)
	require.Nil(t, got)
}

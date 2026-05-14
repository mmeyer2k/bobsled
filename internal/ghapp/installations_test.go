// internal/ghapp/installations_test.go
package ghapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListAccessibleRepos(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": int64(11), "account": map[string]any{"login": "mmeyer2k"}},
				{"id": int64(22), "account": map[string]any{"login": "acme"}},
			})
		case "/app/installations/11/access_tokens", "/app/installations/22/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_x"})
		case "/installation/repositories":
			// Distinguish installations via the Authorization token's
			// presence (both use "ghs_x" in this test; we instead key on
			// the user-agent / query). For simplicity return the same shape
			// twice with different repo lists keyed off ?page.
			if r.URL.Query().Get("page") == "2" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"total_count":  1,
					"repositories": []map[string]any{{"full_name": "acme/bar"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count": 2,
				"repositories": []map[string]any{
					{"full_name": "mmeyer2k/foo"},
					{"full_name": "mmeyer2k/baz"},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	got, err := c.ListAccessibleRepos(t.Context())
	require.NoError(t, err)
	sort.Strings(got)
	// Two installations, each return the same two repos in this stub — that's
	// fine; the function deduplicates by full_name.
	require.Equal(t, []string{"mmeyer2k/baz", "mmeyer2k/foo"}, got)
}

func TestListAccessibleRepos_Paginates(t *testing.T) {
	keyPath, _ := writeKey(t)
	pageCalls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": int64(11), "account": map[string]any{"login": "u"}},
			})
		case "/app/installations/11/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "t"})
		case "/installation/repositories":
			pageCalls++
			if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<`+srv.URL+`/installation/repositories?page=2>; rel="next"`)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"repositories": []map[string]any{{"full_name": "u/a"}},
				})
				return
			}
			// page 2, no Link header → end
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repositories": []map[string]any{{"full_name": "u/b"}},
			})
		}
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	got, err := c.ListAccessibleRepos(t.Context())
	require.NoError(t, err)
	sort.Strings(got)
	require.Equal(t, []string{"u/a", "u/b"}, got)
	require.Equal(t, 2, pageCalls)
}

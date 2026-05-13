// internal/ghapp/token_test.go
package ghapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveInstallation(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/acme/foo/installation", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7777)})
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	id, err := c.ResolveInstallation(t.Context(), "acme/foo")
	require.NoError(t, err)
	require.Equal(t, int64(7777), id)
}

func TestFetchInstallationToken(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.True(t, strings.HasPrefix(r.URL.Path, "/app/installations/12345/access_tokens"))
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_abc"})
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 42, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	tok, err := c.FetchInstallationToken(t.Context(), 12345)
	require.NoError(t, err)
	require.Equal(t, "ghs_abc", tok)
}

func TestFetchInstallationToken_HTTPError(t *testing.T) {
	keyPath, _ := writeKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad creds"}`))
	}))
	defer srv.Close()
	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	_, err := c.FetchInstallationToken(t.Context(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

// internal/tui/poller/runners_test.go
package poller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/stretchr/testify/require"
)

func mkAppKey(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	b := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	p := filepath.Join(t.TempDir(), "app.pem")
	require.NoError(t, os.WriteFile(p, b, 0o600))
	return p
}

func TestRunnersPoller_EmitsOnce(t *testing.T) {
	keyPath := mkAppKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runners":
			w.Header().Set("ETag", `"v1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"runners": []map[string]any{{"id": 1, "name": "bobsled-h1-1"}}})
		}
	}))
	defer srv.Close()
	c := &ghapp.Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emit := make(chan RunnersMsg, 4)
	go RunnersPoller(ctx, c, []string{"acme/foo"}, 10*time.Millisecond, emit)

	select {
	case msg := <-emit:
		require.NoError(t, msg.Err)
		require.Equal(t, "acme/foo", msg.Repo)
		require.Len(t, msg.State.Runners, 1)
		require.Equal(t, `"v1"`, msg.State.ETag)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no message")
	}
}

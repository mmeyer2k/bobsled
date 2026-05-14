// internal/runner/mint_test.go
package runner

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

	"github.com/m-meyer2k/bobsled/internal/state"
	"github.com/stretchr/testify/require"
)

func writeKeyForTest(t *testing.T, dir string) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	b := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	p := filepath.Join(dir, "app.pem")
	require.NoError(t, os.WriteFile(p, b, 0o600))
	return p
}

func TestMint_HappyPath(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeKeyForTest(t, dir)

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(
		"app_id: 1\napp_key_path: "+keyPath+"\nhost_label: h1\n"), 0o600))

	stPath := filepath.Join(dir, "state.yaml")
	require.NoError(t, state.Write(stPath, &state.State{
		Repos:     map[string]state.RepoConfig{"acme/foo": {Labels: []string{"bobsled", "podman"}}},
		Instances: map[int]state.Instance{7: {Repo: "acme/foo"}},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(11)})
		case "/app/installations/11/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runners/generate-jitconfig":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runner":             map[string]any{"id": 1, "name": "bobsled-h1-7"},
				"encoded_jit_config": "ZmFrZQ==",
			})
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cacheRoot := filepath.Join(dir, "cache")
	out := filepath.Join(dir, "jit.json")
	require.NoError(t, Mint(context.Background(), Options{
		ConfigPath: cfgPath, StatePath: stPath, Instance: 7,
		OutputPath: out, CacheRoot: cacheRoot, APIBase: srv.URL,
	}))

	b, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Contains(t, string(b), "ZmFrZQ==")
	target, err := os.Readlink(filepath.Join(cacheRoot, "slots", "7", "current"))
	require.NoError(t, err)
	require.Equal(t, "acme--foo", target)
}

func TestMint_RecoversFrom409Conflict(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeKeyForTest(t, dir)

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(
		"app_id: 1\napp_key_path: "+keyPath+"\nhost_label: h1\n"), 0o600))

	stPath := filepath.Join(dir, "state.yaml")
	require.NoError(t, state.Write(stPath, &state.State{
		Repos:     map[string]state.RepoConfig{"acme/foo": {Labels: []string{"bobsled"}}},
		Instances: map[int]state.Instance{7: {Repo: "acme/foo"}},
	}))

	var jitAttempts int
	var deletedID int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(11)})
		case "/app/installations/11/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runners/generate-jitconfig":
			jitAttempts++
			if jitAttempts == 1 {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"message":"Already exists - A runner with the name *** already exists.","status":"409"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runner":             map[string]any{"id": 99, "name": "bobsled-h1-7"},
				"encoded_jit_config": "ZmFrZQ==",
			})
		case "/repos/acme/foo/actions/runners":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runners": []map[string]any{{"id": 42, "name": "bobsled-h1-7"}},
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

	cacheRoot := filepath.Join(dir, "cache")
	out := filepath.Join(dir, "jit.json")
	require.NoError(t, Mint(context.Background(), Options{
		ConfigPath: cfgPath, StatePath: stPath, Instance: 7,
		OutputPath: out, CacheRoot: cacheRoot, APIBase: srv.URL,
	}))

	require.Equal(t, int64(42), deletedID, "orphan runner should have been deleted")
	require.Equal(t, 2, jitAttempts, "JIT should have been retried once")
}

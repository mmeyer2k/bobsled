// internal/ghapp/jit_test.go
package ghapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateJITConfig(t *testing.T) {
	keyPath, _ := writeKey(t)
	var gotName string
	var gotLabels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(11)})
		case "/app/installations/11/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_xyz"})
		case "/repos/acme/foo/actions/runners/generate-jitconfig":
			require.Equal(t, "token ghs_xyz", r.Header.Get("Authorization"))
			var body struct {
				Name          string   `json:"name"`
				RunnerGroupID int      `json:"runner_group_id"`
				Labels        []string `json:"labels"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			gotName = body.Name
			gotLabels = body.Labels
			require.Equal(t, 1, body.RunnerGroupID)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runner":             map[string]any{"id": 99, "name": body.Name},
				"encoded_jit_config": "ZmFrZQ==",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}
	out, err := c.GenerateJITConfig(t.Context(), "acme/foo", JITRequest{
		Name: "bobsled-h1-7", Labels: []string{"bobsled", "podman"},
	})
	require.NoError(t, err)
	require.Equal(t, "bobsled-h1-7", gotName)
	require.Equal(t, []string{"bobsled", "podman"}, gotLabels)
	require.Equal(t, "ZmFrZQ==", out.EncodedJITConfig)
}

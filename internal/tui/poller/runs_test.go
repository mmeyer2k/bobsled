// internal/tui/poller/runs_test.go
package poller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/stretchr/testify/require"
)

func TestRunsPoller_Emits(t *testing.T) {
	keyPath := mkAppKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/foo/installation":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": int64(7)})
		case "/app/installations/7/access_tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs"})
		case "/repos/acme/foo/actions/runs":
			st := r.URL.Query().Get("status")
			w.Header().Set("ETag", `"e-`+st+`"`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workflow_runs": []map[string]any{
					{"id": 1, "name": "smoke-" + st, "status": st},
				},
			})
		}
	}))
	defer srv.Close()
	c := &ghapp.Client{APIBase: srv.URL, AppID: 1, KeyPath: keyPath, HTTP: srv.Client(), Now: time.Now}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emit := make(chan RunsMsg, 4)
	go RunsPoller(ctx, c, []string{"acme/foo"}, 10*time.Millisecond, emit)

	select {
	case msg := <-emit:
		require.NoError(t, msg.Err)
		require.Equal(t, "acme/foo", msg.Repo)
		require.GreaterOrEqual(t, len(msg.State.Recent), 1)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no message")
	}
}

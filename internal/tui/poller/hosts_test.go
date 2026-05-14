// internal/tui/poller/hosts_test.go
package poller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fakeSSH(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ssh")
	script := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%b' %q\nexit %d\n", stdout, exitCode)
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestProbeHost_Parses(t *testing.T) {
	stdout := "bobsled@1.service loaded active running bobsled GitHub Actions runner slot 1\n" +
		"bobsled@2.service loaded activating start-pre bobsled GitHub Actions runner slot 2\n" +
		"---STATE---\n" +
		"repos:\n  acme/foo: {labels: [bobsled]}\n" +
		"instances:\n  1: {repo: acme/foo}\n  2: {repo: acme/foo}\n"
	fakeSSH(t, stdout, 0)

	st, err := ProbeHost(context.Background(), NewSSHMux(), "bobsled@h1")
	require.NoError(t, err)
	require.True(t, st.Reachable)
	require.Equal(t, "active", st.Slots[1].UnitState)
	require.Equal(t, "activating", st.Slots[2].UnitState)
	require.Equal(t, "acme/foo", st.Slots[1].Repo)
}

func TestProbeHost_SSHFails(t *testing.T) {
	fakeSSH(t, "Connection refused", 255)
	st, err := ProbeHost(context.Background(), NewSSHMux(), "bobsled@h1")
	require.NoError(t, err, "non-zero exit is wrapped, not returned as Go error")
	require.False(t, st.Reachable)
	require.NotEmpty(t, st.LastError)
}

func TestHostsPoller_EmitsOnEachTick(t *testing.T) {
	fakeSSH(t, "---STATE---\n", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emit := make(chan HostsMsg, 4)
	go HostsPoller(ctx, NewSSHMux(), map[string]string{"h1": "bobsled@h1"}, 10*time.Millisecond, emit)

	got := 0
	deadline := time.After(200 * time.Millisecond)
	for got < 3 {
		select {
		case <-emit:
			got++
		case <-deadline:
			t.Fatalf("only got %d ticks", got)
		}
	}
}

// internal/tui/poller/sshmux.go
package poller

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SSHMux returns the args needed to make a multiplexed ssh call. The first
// call to SSHMux(target) lazy-creates a control socket at /tmp/bobsled-tui-<hash>
// and subsequent calls re-use it (ControlPersist keeps the master alive).
type SSHMux struct {
	mu      sync.Mutex
	cleanup map[string]string // target -> control path, for ExitAll
}

func NewSSHMux() *SSHMux { return &SSHMux{cleanup: map[string]string{}} }

// Args returns the slice that should be prepended to any ssh/scp invocation
// for the given target.
func (m *SSHMux) Args(target string) []string {
	cp := m.controlPath(target)
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + cp,
		"-o", "ControlPersist=60",
		"-o", "ConnectTimeout=5",
	}
}

func (m *SSHMux) controlPath(target string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.cleanup[target]; ok {
		return p
	}
	p := filepath.Join(os.TempDir(), fmt.Sprintf("bobsled-tui-%x.sock", hash32(target)))
	m.cleanup[target] = p
	return p
}

// ExitAll asks each master to exit. Idempotent.
func (m *SSHMux) ExitAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.cleanup {
		_ = os.Remove(p)
	}
}

// hash32 is a tiny deterministic hash so control paths are stable but unique.
func hash32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

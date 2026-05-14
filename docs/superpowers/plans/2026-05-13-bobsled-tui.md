# bobsled tui Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `bobsled tui` subcommand: a Bubbletea full-screen TUI that shows a live fleet tree (hosts + slots + runners + workload), supports keypress actions (add slot, add host, drain, remove host, reset cache, gc, view logs) with confirm-modals for destructive ops, and adds two new CLI subcommands (`host add`, `host remove`) that both the TUI and shell can call.

**Architecture:** Bubbletea single-Model program. Three goroutine pollers (hosts via SSH every 2 s; runners via GitHub API every 3 s; workflow runs every 15 s) push messages that the model consumes in `Update`. Actions dispatch as one-shot `tea.Cmd`s that wrap the existing CLI subcommand implementations. ETag-conditional requests on the two GitHub pollers keep the rate-limit footprint small. SSH `ControlMaster` keeps the per-host poll cheap.

**Tech Stack:** Go 1.24+, `charmbracelet/bubbletea`, `charmbracelet/lipgloss`, `charmbracelet/bubbles/viewport`, `charmbracelet/bubbles/textinput`. Standard `net/http` (no GitHub SDK). Shells out to `ssh` for host probing and log streaming.

**Spec:** `docs/superpowers/specs/2026-05-13-bobsled-tui-design.md` is authoritative — re-read sections when a task references them.

**Environment:** Go binary at `/home/mike/.local/go/bin/go` — prefix every go command with `PATH=/home/mike/.local/go/bin:$PATH`. Working dir `/home/mike/Code/bobsled`. Existing test suite must keep passing.

---

## File structure (additions)

```
internal/cli/
    tui.go                      # NEW   — `bobsled tui` cobra wiring
    host_add.go                 # NEW   — `bobsled host add`
    host_remove.go              # NEW   — `bobsled host remove`
    host_bootstrap.go           # MOD   — register the two new host subcommands

internal/inventory/
    mutate.go                   # NEW   — AddHost, RemoveHost, AdjustPool
    mutate_test.go              # NEW
    write.go                    # NEW   — atomic Write(path, *Inventory)
    write_test.go               # NEW

internal/ghapp/
    runs.go                     # NEW   — WorkflowRun + ListWorkflowRuns
    runs_test.go                # NEW
    runners.go                  # MOD   — ETag-aware variant of ListRepoRunners
    runners_test.go             # MOD

internal/tui/
    model.go                    # NEW   — Model, Init, top-level Update
    model_test.go               # NEW
    keys.go                     # NEW   — key bindings + dispatch
    keys_test.go                # NEW
    cursor.go                   # NEW   — tree cursor (host header vs slot)
    cursor_test.go              # NEW
    rows.go                     # NEW   — pure data-to-rows (testable without ANSI)
    rows_test.go                # NEW
    layout.go                   # NEW   — lipgloss styling + frame assembly
    layout_test.go              # NEW   — golden-file tests (ANSI-stripped)
    modal.go                    # NEW   — confirm modal + inline prompt
    modal_test.go               # NEW
    actions.go                  # NEW   — action dispatch (tea.Cmds)
    actions_test.go             # NEW
    pager.go                    # NEW   — journalctl pager subview
    poller/
        hosts.go                # NEW   — SSH host probe poller
        hosts_test.go           # NEW
        runners.go              # NEW   — runners poller
        runners_test.go         # NEW
        runs.go                 # NEW   — workflow runs poller
        runs_test.go            # NEW
        sshmux.go               # NEW   — ssh ControlMaster helper
```

---

## Phase 0 — Dependencies

### Task 1: Add Bubbletea + lipgloss + bubbles

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add deps**

```bash
cd /home/mike/Code/bobsled
PATH=/home/mike/.local/go/bin:$PATH go get github.com/charmbracelet/bubbletea@latest
PATH=/home/mike/.local/go/bin:$PATH go get github.com/charmbracelet/lipgloss@latest
PATH=/home/mike/.local/go/bin:$PATH go get github.com/charmbracelet/bubbles@latest
PATH=/home/mike/.local/go/bin:$PATH go mod tidy
```

- [ ] **Step 2: Verify go.mod lists all three as direct (after we use them they'll become direct; for now they may show `// indirect`)**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
```

Expected: builds clean (the deps aren't imported yet, so they may sit as indirect).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add bubbletea, lipgloss, bubbles for tui"
```

---

## Phase 1 — Inventory mutation helpers

### Task 2: `internal/inventory/mutate.go` — AddHost, RemoveHost, AdjustPool

**Files:**
- Create: `internal/inventory/mutate.go`
- Create: `internal/inventory/mutate_test.go`

- [ ] **Step 1: Write the test**

```go
// internal/inventory/mutate_test.go
package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleInv() *Inventory {
	return &Inventory{
		GitHub: GitHubAuth{AppID: 1, AppKey: "/tmp/k"},
		Hosts: map[string]Host{
			"h1": {SSH: "bobsled@h1", BootstrapSSH: "mike@h1", Capacity: 4},
		},
		Pools: []Pool{
			{Repo: "acme/foo", Count: 2, Labels: []string{"bobsled"}, Spread: []string{"h1"}},
		},
	}
}

func TestAddHost_New(t *testing.T) {
	out, err := AddHost(sampleInv(), "h2", Host{SSH: "bobsled@h2", BootstrapSSH: "mike@h2", Capacity: 8})
	require.NoError(t, err)
	require.Equal(t, 8, out.Hosts["h2"].Capacity)
	require.Equal(t, "bobsled@h1", out.Hosts["h1"].SSH, "existing hosts preserved")
}

func TestAddHost_AlreadyExists(t *testing.T) {
	_, err := AddHost(sampleInv(), "h1", Host{SSH: "x", Capacity: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestRemoveHost(t *testing.T) {
	inv := sampleInv()
	inv.Pools[0].Spread = []string{"h1", "h2"}
	inv.Hosts["h2"] = Host{SSH: "bobsled@h2", Capacity: 4}

	out, err := RemoveHost(inv, "h2")
	require.NoError(t, err)
	require.NotContains(t, out.Hosts, "h2")
	require.Equal(t, []string{"h1"}, out.Pools[0].Spread, "h2 removed from spread")
}

func TestRemoveHost_PrunesEmptyPool(t *testing.T) {
	inv := sampleInv()
	out, err := RemoveHost(inv, "h1")
	require.NoError(t, err)
	require.NotContains(t, out.Hosts, "h1")
	require.Empty(t, out.Pools, "pool with empty spread is pruned")
}

func TestRemoveHost_NotFound(t *testing.T) {
	_, err := RemoveHost(sampleInv(), "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestAdjustPool_IncreaseExisting(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/foo", +3, nil)
	require.NoError(t, err)
	require.Equal(t, 5, out.Pools[0].Count)
}

func TestAdjustPool_DecreaseToZeroPrunes(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/foo", -2, nil)
	require.NoError(t, err)
	require.Empty(t, out.Pools)
}

func TestAdjustPool_CreatesNewPool(t *testing.T) {
	out, err := AdjustPool(sampleInv(), "acme/new", +1, []string{"h1"})
	require.NoError(t, err)
	require.Len(t, out.Pools, 2)
	pool := out.Pools[1]
	require.Equal(t, "acme/new", pool.Repo)
	require.Equal(t, 1, pool.Count)
	require.Equal(t, []string{"h1"}, pool.Spread)
}

func TestAdjustPool_NewPoolWithoutHostsErrors(t *testing.T) {
	_, err := AdjustPool(sampleInv(), "acme/new", +1, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spread")
}
```

- [ ] **Step 2: Run, expect fail**

```bash
cd /home/mike/Code/bobsled && PATH=/home/mike/.local/go/bin:$PATH go test ./internal/inventory/...
```

- [ ] **Step 3: Implement**

```go
// internal/inventory/mutate.go
package inventory

import "fmt"

// AddHost returns a copy of inv with the given host added. Errors if the host
// name is already present. Does not mutate inv.
func AddHost(inv *Inventory, name string, h Host) (*Inventory, error) {
	if _, exists := inv.Hosts[name]; exists {
		return nil, fmt.Errorf("host %q already exists", name)
	}
	out := cloneInventory(inv)
	out.Hosts[name] = h
	return out, nil
}

// RemoveHost returns a copy of inv without the given host. Also drops the host
// from any pool's spread, and prunes pools whose spread becomes empty.
func RemoveHost(inv *Inventory, name string) (*Inventory, error) {
	if _, exists := inv.Hosts[name]; !exists {
		return nil, fmt.Errorf("host %q not found", name)
	}
	out := cloneInventory(inv)
	delete(out.Hosts, name)
	pools := make([]Pool, 0, len(out.Pools))
	for _, p := range out.Pools {
		spread := make([]string, 0, len(p.Spread))
		for _, h := range p.Spread {
			if h != name {
				spread = append(spread, h)
			}
		}
		if len(spread) == 0 {
			continue
		}
		p.Spread = spread
		pools = append(pools, p)
	}
	out.Pools = pools
	return out, nil
}

// AdjustPool changes the count of an existing pool by delta, or creates a new
// pool when none exists for repo. New pools require a non-empty spread.
// A delta that brings count to 0 or below prunes the pool entirely.
func AdjustPool(inv *Inventory, repo string, delta int, spread []string) (*Inventory, error) {
	out := cloneInventory(inv)
	for i := range out.Pools {
		if out.Pools[i].Repo == repo {
			out.Pools[i].Count += delta
			if out.Pools[i].Count <= 0 {
				out.Pools = append(out.Pools[:i], out.Pools[i+1:]...)
			}
			return out, nil
		}
	}
	if len(spread) == 0 {
		return nil, fmt.Errorf("creating new pool for %q requires a non-empty spread", repo)
	}
	out.Pools = append(out.Pools, Pool{
		Repo:   repo,
		Count:  delta,
		Labels: []string{"self-hosted", "linux", "x64", "bobsled", "podman"},
		Spread: append([]string(nil), spread...),
	})
	return out, nil
}

func cloneInventory(inv *Inventory) *Inventory {
	out := &Inventory{
		GitHub: inv.GitHub,
		Hosts:  make(map[string]Host, len(inv.Hosts)),
		Pools:  make([]Pool, len(inv.Pools)),
	}
	for k, v := range inv.Hosts {
		out.Hosts[k] = v
	}
	for i, p := range inv.Pools {
		p.Labels = append([]string(nil), p.Labels...)
		p.Spread = append([]string(nil), p.Spread...)
		out.Pools[i] = p
	}
	return out
}
```

- [ ] **Step 4: Run, verify pass**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/inventory/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/mutate.go internal/inventory/mutate_test.go
git commit -m "feat(inventory): AddHost / RemoveHost / AdjustPool helpers"
```

---

### Task 3: `internal/inventory/write.go` — atomic write

**Files:**
- Create: `internal/inventory/write.go`
- Create: `internal/inventory/write_test.go`

- [ ] **Step 1: Test**

```go
// internal/inventory/write_test.go
package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	in := sampleInv()
	require.NoError(t, Write(path, in))

	got, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, in.GitHub, got.GitHub)
	require.Equal(t, in.Hosts, got.Hosts)
	require.Equal(t, in.Pools, got.Pools)
}

func TestWrite_NoLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yaml")
	require.NoError(t, Write(path, sampleInv()))
	ents, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, ents, 1)
	require.Equal(t, "inv.yaml", ents[0].Name())
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/inventory/...
```

- [ ] **Step 3: Implement**

```go
// internal/inventory/write.go
package inventory

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Write atomically replaces path with the marshalled inventory via a same-dir
// rename(2). Concurrent writers may race; document "don't run two host add/
// remove invocations at the same time" — flock is YAGNI here.
func Write(path string, inv *Inventory) error {
	b, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".inventory.*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		cleanup()
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/inventory/... -v
git add internal/inventory/write.go internal/inventory/write_test.go
git commit -m "feat(inventory): atomic Write(path, *Inventory)"
```

---

## Phase 2 — ghapp: workflow runs + ETag

### Task 4: `internal/ghapp/runs.go` — list workflow runs

**Files:**
- Create: `internal/ghapp/runs.go`
- Create: `internal/ghapp/runs_test.go`

- [ ] **Step 1: Test**

```go
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
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/ghapp/...
```

- [ ] **Step 3: Implement**

```go
// internal/ghapp/runs.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WorkflowRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`       // queued | in_progress | completed
	Conclusion   string    `json:"conclusion"`   // success | failure | cancelled | ""
	HeadBranch   string    `json:"head_branch"`
	RunStartedAt time.Time `json:"run_started_at"`
}

// ListWorkflowRuns returns workflow runs filtered by status ("queued",
// "in_progress", or "" for all recent). Passing a non-empty etag sends
// If-None-Match; on 304 the returned slice is nil and the returned etag echoes
// the input (callers should treat that as "no change").
func (c *Client) ListWorkflowRuns(ctx context.Context, repo, status, etag string) ([]WorkflowRun, string, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{}
	q.Set("per_page", "10")
	if status != "" {
		q.Set("status", status)
	}
	u := fmt.Sprintf("%s/repos/%s/actions/runs?%s", strings.TrimRight(c.APIBase, "/"), repo, q.Encode())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	newETag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		return nil, newETag, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("list runs: %s: %s", resp.Status, b)
	}
	var out struct {
		Runs []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Runs, newETag, nil
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/ghapp/... -v
git add internal/ghapp/runs.go internal/ghapp/runs_test.go
git commit -m "feat(ghapp): ListWorkflowRuns with ETag support"
```

---

### Task 5: `internal/ghapp/runners.go` — ETag-aware variant

**Files:**
- Modify: `internal/ghapp/runners.go`
- Modify: `internal/ghapp/runners_test.go`

- [ ] **Step 1: Add an ETag test**

Append to `internal/ghapp/runners_test.go`:

```go
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
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/ghapp/... -run ETag
```

- [ ] **Step 3: Add `ListRepoRunnersETag` to `runners.go`**

Append to `internal/ghapp/runners.go`:

```go
// ListRepoRunnersETag is the ETag-aware variant of ListRepoRunners. Pass an
// empty etag for the first call; pass the returned etag on subsequent calls.
// On 304 (no change) the returned slice is nil and the returned etag echoes
// the input.
func (c *Client) ListRepoRunnersETag(ctx context.Context, repo, etag string) ([]RunnerRef, string, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runners?per_page=100", strings.TrimRight(c.APIBase, "/"), repo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	newETag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		return nil, newETag, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("list runners: %s: %s", resp.Status, b)
	}
	var out struct {
		Runners []RunnerRef `json:"runners"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Runners, newETag, nil
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/ghapp/... -v
git add internal/ghapp/runners.go internal/ghapp/runners_test.go
git commit -m "feat(ghapp): ListRepoRunnersETag for conditional polling"
```

---

## Phase 3 — SSH multiplexing helper

### Task 6: `internal/tui/poller/sshmux.go`

**Files:**
- Create: `internal/tui/poller/sshmux.go`

- [ ] **Step 1: Implement** (no test; this is a thin wrapper)

```go
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
```

- [ ] **Step 2: Build smoke check**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/poller/sshmux.go
git commit -m "feat(tui): SSH ControlMaster mux helper"
```

---

## Phase 4 — Host poller

### Task 7: `internal/tui/poller/hosts.go` — probe a single host

**Files:**
- Create: `internal/tui/poller/hosts.go`
- Create: `internal/tui/poller/hosts_test.go`

- [ ] **Step 1: Test** (using fake `ssh` on PATH, same pattern as `internal/ssh` tests)

```go
// internal/tui/poller/hosts_test.go
package poller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeSSH(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ssh")
	script := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s' %q\nexit %d\n", stdout, exitCode)
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestProbeHost_Parses(t *testing.T) {
	// systemctl list-units output:
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
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/poller/hosts.go
package poller

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/m-meyer2k/bobsled/internal/state"
	"gopkg.in/yaml.v3"
)

type SlotState struct {
	N         int
	UnitState string
	Repo      string
	Container string
	StartedAt time.Time
}

type HostState struct {
	Name       string
	Slots      map[int]SlotState
	Capacity   int
	Reachable  bool
	LastError  string
	LastUpdate time.Time
}

// ProbeHost runs one combined SSH command to fetch units + state.yaml in a
// single round-trip. Non-zero ssh exits set Reachable=false and stash the
// error in LastError; the function itself returns an error only on truly
// unexpected failures (e.g. exec failed locally).
func ProbeHost(ctx context.Context, mux *SSHMux, target string) (*HostState, error) {
	args := append(mux.Args(target), target,
		`systemctl --user list-units 'bobsled@*' --all --no-legend --plain --no-pager 2>/dev/null; `+
			`echo '---STATE---'; cat state.yaml 2>/dev/null`)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	st := &HostState{
		Name:       target,
		Slots:      map[int]SlotState{},
		LastUpdate: time.Now(),
	}
	if err != nil {
		st.Reachable = false
		st.LastError = fmt.Sprintf("%v: %s", err, strings.TrimSpace(stderr.String()))
		return st, nil
	}
	st.Reachable = true
	parts := strings.SplitN(stdout.String(), "---STATE---", 2)
	parseUnits(parts[0], st)
	if len(parts) == 2 {
		parseState(parts[1], st)
	}
	return st, nil
}

func parseUnits(s string, st *HostState) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(f[0], "bobsled@") {
			continue
		}
		var n int
		_, _ = fmt.Sscanf(f[0], "bobsled@%d.service", &n)
		ss := st.Slots[n]
		ss.N = n
		ss.UnitState = f[2] // active / activating / failed / inactive
		st.Slots[n] = ss
	}
}

func parseState(s string, st *HostState) {
	var parsed state.State
	if err := yaml.Unmarshal([]byte(s), &parsed); err != nil || parsed.Instances == nil {
		return
	}
	for n, inst := range parsed.Instances {
		ss := st.Slots[n]
		ss.N = n
		ss.Repo = inst.Repo
		st.Slots[n] = ss
	}
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/... -v
git add internal/tui/poller/hosts.go internal/tui/poller/hosts_test.go
git commit -m "feat(tui/poller): probe a single host's units + state.yaml"
```

---

### Task 8: `internal/tui/poller/hosts_loop.go` — periodic emit

**Files:**
- Modify: `internal/tui/poller/hosts.go` (append the loop function)
- Modify: `internal/tui/poller/hosts_test.go` (append a test)

- [ ] **Step 1: Test the loop function emits on a synthetic clock**

Append to `internal/tui/poller/hosts_test.go`:

```go
func TestHostsPoller_EmitsOnEachTick(t *testing.T) {
	fakeSSH(t, "---STATE---\n", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emit := make(chan HostsMsg, 4)
	go HostsPoller(ctx, NewSSHMux(), []string{"bobsled@h1"}, 10*time.Millisecond, emit)

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
```

Add `import "time"` if it's not already there.

- [ ] **Step 2: Implement the loop**

Append to `internal/tui/poller/hosts.go`:

```go
type HostsMsg struct {
	Host  string
	State *HostState
	Err   error
}

// HostsPoller probes each target on an interval and sends results to emit.
// One goroutine per target so a slow host doesn't block the others. Stops on
// ctx done.
func HostsPoller(ctx context.Context, mux *SSHMux, targets []string, interval time.Duration, emit chan<- HostsMsg) {
	for _, t := range targets {
		go hostLoop(ctx, mux, t, interval, emit)
	}
	<-ctx.Done()
}

func hostLoop(ctx context.Context, mux *SSHMux, target string, interval time.Duration, emit chan<- HostsMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	// First probe immediately.
	for {
		st, err := ProbeHost(ctx, mux, target)
		select {
		case <-ctx.Done():
			return
		case emit <- HostsMsg{Host: target, State: st, Err: err}:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
```

- [ ] **Step 3: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/... -v
git add internal/tui/poller/
git commit -m "feat(tui/poller): periodic HostsPoller"
```

---

## Phase 5 — Runners and runs pollers

### Task 9: `internal/tui/poller/runners.go`

**Files:**
- Create: `internal/tui/poller/runners.go`
- Create: `internal/tui/poller/runners_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/poller/runners_test.go
package poller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

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
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/poller/runners.go
package poller

import (
	"context"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
)

type RepoRunners struct {
	Runners []ghapp.RunnerRef
	ETag    string
	Updated time.Time
}

type RunnersMsg struct {
	Repo  string
	State *RepoRunners
	Err   error
}

// RunnersPoller polls each repo's runners endpoint with ETag conditional
// requests. One goroutine per repo.
func RunnersPoller(ctx context.Context, c *ghapp.Client, repos []string, interval time.Duration, emit chan<- RunnersMsg) {
	for _, r := range repos {
		go runnersLoop(ctx, c, r, interval, emit)
	}
	<-ctx.Done()
}

func runnersLoop(ctx context.Context, c *ghapp.Client, repo string, interval time.Duration, emit chan<- RunnersMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var etag string
	var lastList []ghapp.RunnerRef
	for {
		runners, newETag, err := c.ListRepoRunnersETag(ctx, repo, etag)
		msg := RunnersMsg{Repo: repo, Err: err}
		if err == nil {
			if runners == nil {
				// 304: keep last list
				msg.State = &RepoRunners{Runners: lastList, ETag: newETag, Updated: time.Now()}
			} else {
				lastList = runners
				etag = newETag
				msg.State = &RepoRunners{Runners: runners, ETag: newETag, Updated: time.Now()}
			}
		}
		select {
		case <-ctx.Done():
			return
		case emit <- msg:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/... -v
git add internal/tui/poller/runners.go internal/tui/poller/runners_test.go
git commit -m "feat(tui/poller): runners poller with ETag"
```

---

### Task 10: `internal/tui/poller/runs.go`

**Files:**
- Create: `internal/tui/poller/runs.go`
- Create: `internal/tui/poller/runs_test.go`

- [ ] **Step 1: Test**

```go
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
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/poller/runs.go
package poller

import (
	"context"
	"sort"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
)

type RepoRuns struct {
	Queued     []ghapp.WorkflowRun
	InProgress []ghapp.WorkflowRun
	Recent     []ghapp.WorkflowRun
	ETag       string // composite (queued|in_progress|recent)
	Updated    time.Time
}

type RunsMsg struct {
	Repo  string
	State *RepoRuns
	Err   error
}

func RunsPoller(ctx context.Context, c *ghapp.Client, repos []string, interval time.Duration, emit chan<- RunsMsg) {
	for _, r := range repos {
		go runsLoop(ctx, c, r, interval, emit)
	}
	<-ctx.Done()
}

func runsLoop(ctx context.Context, c *ghapp.Client, repo string, interval time.Duration, emit chan<- RunsMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var (
		etagQ, etagP, etagR     string
		lastQ, lastP, lastR     []ghapp.WorkflowRun
	)
	for {
		queued, newQ, errQ := c.ListWorkflowRuns(ctx, repo, "queued", etagQ)
		inprog, newP, errP := c.ListWorkflowRuns(ctx, repo, "in_progress", etagP)
		recent, newR, errR := c.ListWorkflowRuns(ctx, repo, "", etagR)
		err := firstErr(errQ, errP, errR)
		if errQ == nil && queued == nil {
			queued = lastQ
		} else if errQ == nil {
			lastQ = queued
			etagQ = newQ
		}
		if errP == nil && inprog == nil {
			inprog = lastP
		} else if errP == nil {
			lastP = inprog
			etagP = newP
		}
		if errR == nil && recent == nil {
			recent = lastR
		} else if errR == nil {
			lastR = recent
			etagR = newR
		}
		sort.Slice(recent, func(i, j int) bool {
			return recent[i].RunStartedAt.After(recent[j].RunStartedAt)
		})
		msg := RunsMsg{Repo: repo, Err: err}
		if err == nil {
			msg.State = &RepoRuns{
				Queued: queued, InProgress: inprog, Recent: recent,
				ETag: etagQ + "|" + etagP + "|" + etagR, Updated: time.Now(),
			}
		}
		select {
		case <-ctx.Done():
			return
		case emit <- msg:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/poller/... -v
git add internal/tui/poller/runs.go internal/tui/poller/runs_test.go
git commit -m "feat(tui/poller): workflow runs poller (queued + in_progress + recent)"
```

---

## Phase 6 — TUI Model and cursor

### Task 11: `internal/tui/cursor.go` — tree navigation

**Files:**
- Create: `internal/tui/cursor.go`
- Create: `internal/tui/cursor_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/cursor_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func twoHosts() map[string]*poller.HostState {
	return map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}, 2: {N: 2}}},
		"h2": {Name: "h2", Slots: map[int]poller.SlotState{1: {N: 1}}},
	}
}

func TestCursor_MovesThroughTree(t *testing.T) {
	expanded := map[string]bool{"h1": true, "h2": true}
	hosts := twoHosts()

	c := FirstCursor(hosts, expanded)
	require.Equal(t, "h1", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, CursorSlot, c.Kind)
	require.Equal(t, 1, c.Slot)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, 2, c.Slot)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorSlot, c.Kind)
	require.Equal(t, 1, c.Slot)

	// Past the end: stays put
	last := c
	c = NextCursor(c, hosts, expanded)
	require.Equal(t, last, c)
}

func TestCursor_SkipsCollapsedSlots(t *testing.T) {
	expanded := map[string]bool{"h1": false, "h2": true}
	hosts := twoHosts()

	c := FirstCursor(hosts, expanded)
	require.Equal(t, "h1", c.Host)
	require.Equal(t, CursorHost, c.Kind)

	c = NextCursor(c, hosts, expanded)
	require.Equal(t, "h2", c.Host)
	require.Equal(t, CursorHost, c.Kind)
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/cursor.go
package tui

import (
	"sort"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type CursorKind int

const (
	CursorHost CursorKind = iota
	CursorSlot
)

type Cursor struct {
	Host string
	Kind CursorKind
	Slot int // when Kind==CursorSlot
}

// FirstCursor returns the cursor pointing at the first host header.
func FirstCursor(hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	if len(names) == 0 {
		return Cursor{}
	}
	return Cursor{Host: names[0], Kind: CursorHost}
}

// NextCursor returns the cursor one row down. Out-of-tree → returns input unchanged.
func NextCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	for i, name := range names {
		switch {
		case c.Host == name && c.Kind == CursorHost:
			if expanded[name] {
				slots := sortedSlotNums(hosts[name])
				if len(slots) > 0 {
					return Cursor{Host: name, Kind: CursorSlot, Slot: slots[0]}
				}
			}
			if i+1 < len(names) {
				return Cursor{Host: names[i+1], Kind: CursorHost}
			}
			return c
		case c.Host == name && c.Kind == CursorSlot:
			slots := sortedSlotNums(hosts[name])
			idx := sort.SearchInts(slots, c.Slot)
			if idx+1 < len(slots) {
				return Cursor{Host: name, Kind: CursorSlot, Slot: slots[idx+1]}
			}
			if i+1 < len(names) {
				return Cursor{Host: names[i+1], Kind: CursorHost}
			}
			return c
		}
	}
	return c
}

// PrevCursor returns the cursor one row up. Out-of-tree → returns input unchanged.
func PrevCursor(c Cursor, hosts map[string]*poller.HostState, expanded map[string]bool) Cursor {
	names := sortedHostNames(hosts)
	for i, name := range names {
		switch {
		case c.Host == name && c.Kind == CursorHost:
			if i == 0 {
				return c
			}
			prev := names[i-1]
			if expanded[prev] {
				slots := sortedSlotNums(hosts[prev])
				if len(slots) > 0 {
					return Cursor{Host: prev, Kind: CursorSlot, Slot: slots[len(slots)-1]}
				}
			}
			return Cursor{Host: prev, Kind: CursorHost}
		case c.Host == name && c.Kind == CursorSlot:
			slots := sortedSlotNums(hosts[name])
			idx := sort.SearchInts(slots, c.Slot)
			if idx == 0 {
				return Cursor{Host: name, Kind: CursorHost}
			}
			return Cursor{Host: name, Kind: CursorSlot, Slot: slots[idx-1]}
		}
	}
	return c
}

func sortedHostNames(m map[string]*poller.HostState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSlotNums(h *poller.HostState) []int {
	if h == nil {
		return nil
	}
	out := make([]int, 0, len(h.Slots))
	for k := range h.Slots {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/cursor.go internal/tui/cursor_test.go
git commit -m "feat(tui): cursor with tree-aware next/prev navigation"
```

---

### Task 12: `internal/tui/model.go` — Model + Init

**Files:**
- Create: `internal/tui/model.go`

- [ ] **Step 1: Implement** (test is in next task because Update is where tests live)

```go
// internal/tui/model.go
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

const (
	hostsInterval   = 2 * time.Second
	runnersInterval = 3 * time.Second
	runsInterval    = 15 * time.Second
)

type Model struct {
	Inv     *inventory.Inventory
	Client  *ghapp.Client
	Mux     *poller.SSHMux
	Hosts   map[string]*poller.HostState
	Runners map[string]*poller.RepoRunners
	Runs    map[string]*poller.RepoRuns
	Errs    map[string]string // source label → last error

	Cursor    Cursor
	Expanded  map[string]bool
	Modal     *Modal
	Inline    *InlinePrompt
	StatusLog *ringBuffer
	Flash     *flash
	Paused    bool
	Width     int
	Height    int
}

func New(inv *inventory.Inventory, c *ghapp.Client) Model {
	expanded := make(map[string]bool, len(inv.Hosts))
	for name := range inv.Hosts {
		expanded[name] = true
	}
	return Model{
		Inv:       inv,
		Client:    c,
		Mux:       poller.NewSSHMux(),
		Hosts:     map[string]*poller.HostState{},
		Runners:   map[string]*poller.RepoRunners{},
		Runs:      map[string]*poller.RepoRuns{},
		Errs:      map[string]string{},
		Expanded:  expanded,
		StatusLog: newRingBuffer(5),
	}
}

func (m Model) Init() tea.Cmd {
	// Pollers run as goroutines; we set them up via a Cmd that returns a
	// listener-cmd to keep pumping the channel into the Bubbletea event loop.
	return tea.Batch(
		startHostsPoller(m),
		startRunnersPoller(m),
		startRunsPoller(m),
	)
}

// Helpers used by Init.

func startHostsPoller(m Model) tea.Cmd {
	targets := make([]string, 0, len(m.Inv.Hosts))
	for _, h := range m.Inv.Hosts {
		targets = append(targets, h.SSH)
	}
	ch := make(chan poller.HostsMsg, 32)
	go poller.HostsPoller(programCtx(), m.Mux, targets, hostsInterval, ch)
	return waitForHostsMsg(ch)
}

func startRunnersPoller(m Model) tea.Cmd {
	repos := poolRepos(m.Inv)
	ch := make(chan poller.RunnersMsg, 32)
	go poller.RunnersPoller(programCtx(), m.Client, repos, runnersInterval, ch)
	return waitForRunnersMsg(ch)
}

func startRunsPoller(m Model) tea.Cmd {
	repos := poolRepos(m.Inv)
	ch := make(chan poller.RunsMsg, 32)
	go poller.RunsPoller(programCtx(), m.Client, repos, runsInterval, ch)
	return waitForRunsMsg(ch)
}

func waitForHostsMsg(ch chan poller.HostsMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return hostsTickMsg{m, ch}
	}
}
func waitForRunnersMsg(ch chan poller.RunnersMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return runnersTickMsg{m, ch}
	}
}
func waitForRunsMsg(ch chan poller.RunsMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return runsTickMsg{m, ch}
	}
}

type hostsTickMsg struct {
	M  poller.HostsMsg
	Ch chan poller.HostsMsg
}
type runnersTickMsg struct {
	M  poller.RunnersMsg
	Ch chan poller.RunnersMsg
}
type runsTickMsg struct {
	M  poller.RunsMsg
	Ch chan poller.RunsMsg
}

func poolRepos(inv *inventory.Inventory) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range inv.Pools {
		if !seen[p.Repo] {
			seen[p.Repo] = true
			out = append(out, p.Repo)
		}
	}
	return out
}
```

- [ ] **Step 2: Add the context-singleton helper**

Append to `internal/tui/model.go`:

```go
import "context"

var pCtx context.Context

func programCtx() context.Context {
	if pCtx == nil {
		pCtx = context.Background()
	}
	return pCtx
}

// SetContext is called by the TUI subcommand entry point before starting the
// Bubbletea program. Allows external lifecycle to cancel pollers.
func SetContext(ctx context.Context) { pCtx = ctx }
```

(Merge the new `import "context"` into the existing imports block.)

- [ ] **Step 3: Add the ring buffer + flash helpers**

Append to `internal/tui/model.go`:

```go
type ringBuffer struct {
	cap   int
	lines []string
}

func newRingBuffer(cap int) *ringBuffer { return &ringBuffer{cap: cap} }
func (r *ringBuffer) Push(s string) {
	r.lines = append(r.lines, s)
	if len(r.lines) > r.cap {
		r.lines = r.lines[len(r.lines)-r.cap:]
	}
}
func (r *ringBuffer) Lines() []string { return append([]string(nil), r.lines...) }

type flash struct {
	Text    string
	IsError bool
	Until   time.Time
}
```

- [ ] **Step 4: Build smoke**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
```

(Update will be empty / panicking right now; we add it next.)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): Model + Init + poller cmd plumbing"
```

---

### Task 13: `internal/tui/model.go` — Update for poller messages

**Files:**
- Modify: `internal/tui/model.go` (append Update function)
- Create: `internal/tui/model_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/model_test.go
package tui

import (
	"testing"
	"time"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	inv := &inventory.Inventory{
		Hosts: map[string]inventory.Host{
			"h1": {SSH: "bobsled@h1", Capacity: 4},
		},
		Pools: []inventory.Pool{{Repo: "acme/foo", Count: 1, Spread: []string{"h1"}}},
	}
	return New(inv, nil)
}

func TestUpdate_HostsTickStoresState(t *testing.T) {
	m := newTestModel(t)
	st := &poller.HostState{
		Name: "bobsled@h1", Reachable: true,
		Slots: map[int]poller.SlotState{1: {N: 1, UnitState: "active", Repo: "acme/foo"}},
		LastUpdate: time.Now(),
	}
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "bobsled@h1", State: st}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Hosts["bobsled@h1"])
	require.Equal(t, "active", mm.Hosts["bobsled@h1"].Slots[1].UnitState)
}

func TestUpdate_RunnersTickStoresState(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(runnersTickMsg{M: poller.RunnersMsg{
		Repo: "acme/foo",
		State: &poller.RepoRunners{},
	}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Runners["acme/foo"])
}

func TestUpdate_RecordsErrorBySource(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "bobsled@h1", Err: assertErr("boom")}})
	mm := mNew.(Model)
	require.Contains(t, mm.Errs["hosts/bobsled@h1"], "boom")
}

type stringErr string
func (e stringErr) Error() string { return string(e) }
func assertErr(s string) error    { return stringErr(s) }
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Append `Update` to `internal/tui/model.go`**

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = v.Width, v.Height
		return m, nil

	case hostsTickMsg:
		if v.M.Err != nil {
			m.Errs["hosts/"+v.M.Host] = v.M.Err.Error()
		} else {
			delete(m.Errs, "hosts/"+v.M.Host)
			m.Hosts[v.M.Host] = v.M.State
		}
		return m, waitForHostsMsg(v.Ch)

	case runnersTickMsg:
		if v.M.Err != nil {
			m.Errs["runners/"+v.M.Repo] = v.M.Err.Error()
		} else {
			delete(m.Errs, "runners/"+v.M.Repo)
			m.Runners[v.M.Repo] = v.M.State
		}
		return m, waitForRunnersMsg(v.Ch)

	case runsTickMsg:
		if v.M.Err != nil {
			m.Errs["runs/"+v.M.Repo] = v.M.Err.Error()
		} else {
			delete(m.Errs, "runs/"+v.M.Repo)
			m.Runs[v.M.Repo] = v.M.State
		}
		return m, waitForRunsMsg(v.Ch)
	}
	return m, nil
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat(tui): Update handles poller ticks and records errors per source"
```

---

### Task 14: `internal/tui/keys.go` — key bindings (nav only)

**Files:**
- Create: `internal/tui/keys.go`
- Create: `internal/tui/keys_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/keys_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func modelWithTwoHosts(t *testing.T) Model {
	t.Helper()
	inv := &inventory.Inventory{
		Hosts: map[string]inventory.Host{
			"h1": {SSH: "bobsled@h1", Capacity: 4},
			"h2": {SSH: "bobsled@h2", Capacity: 4},
		},
	}
	m := New(inv, nil)
	m.Hosts["h1"] = &poller.HostState{Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}, 2: {N: 2}}}
	m.Hosts["h2"] = &poller.HostState{Name: "h2", Slots: map[int]poller.SlotState{1: {N: 1}}}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)
	return m
}

func TestKey_J_MovesCursorDown(t *testing.T) {
	m := modelWithTwoHosts(t)
	require.Equal(t, "h1", m.Cursor.Host)
	require.Equal(t, CursorHost, m.Cursor.Kind)
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mm := mNew.(Model)
	require.Equal(t, CursorSlot, mm.Cursor.Kind)
}

func TestKey_K_MovesCursorUp(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h2", Kind: CursorHost}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	mm := mNew.(Model)
	require.Equal(t, "h1", mm.Cursor.Host)
}

func TestKey_Enter_TogglesExpand(t *testing.T) {
	m := modelWithTwoHosts(t)
	require.True(t, m.Expanded["h1"])
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.False(t, mNew.(Model).Expanded["h1"])
}

func TestKey_Q_Quits(t *testing.T) {
	m := modelWithTwoHosts(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd, "q must return tea.Quit")
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement `keys.go` and merge into `Update`**

Create `internal/tui/keys.go`:

```go
// internal/tui/keys.go
package tui

import tea "github.com/charmbracelet/bubbletea"

// handleKey returns the updated model and an optional cmd. Keys that touch
// only model state (nav, toggle expand) return nil cmd.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		if m.Cursor.Kind == CursorHost {
			m.Expanded[m.Cursor.Host] = !m.Expanded[m.Cursor.Host]
		}
		return m, nil
	case tea.KeyUp:
		m.Cursor = PrevCursor(m.Cursor, m.Hosts, m.Expanded)
		return m, nil
	case tea.KeyDown:
		m.Cursor = NextCursor(m.Cursor, m.Hosts, m.Expanded)
		return m, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return m, nil
	}
	switch msg.Runes[0] {
	case 'q':
		return m, tea.Quit
	case 'j':
		m.Cursor = NextCursor(m.Cursor, m.Hosts, m.Expanded)
	case 'k':
		m.Cursor = PrevCursor(m.Cursor, m.Hosts, m.Expanded)
	}
	return m, nil
}
```

Modify the top of `Update` in `internal/tui/model.go` to dispatch KeyMsg:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(v)
	case tea.WindowSizeMsg:
		// ... existing
	}
	// ...
}
```

(Keep the rest of the existing cases.)

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/keys.go internal/tui/keys_test.go internal/tui/model.go
git commit -m "feat(tui): nav keys j/k/⏎/q + ctrl-c"
```

---

## Phase 7 — Rendering

### Task 15: `internal/tui/rows.go` — data → rows (no styling)

**Files:**
- Create: `internal/tui/rows.go`
- Create: `internal/tui/rows_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/rows_test.go
package tui

import (
	"testing"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func TestBuildRows_HostThenSlots(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Reachable: true, Capacity: 4, Slots: map[int]poller.SlotState{
			1: {N: 1, UnitState: "active", Repo: "acme/foo"},
			2: {N: 2, UnitState: "activating", Repo: "acme/foo"},
		}},
	}
	runners := map[string]*poller.RepoRunners{
		"acme/foo": {Runners: []ghapp.RunnerRef{{Name: "bobsled-h1-1"}}},
	}
	expanded := map[string]bool{"h1": true}

	rows := BuildRows(hosts, runners, expanded)
	require.Equal(t, RowHost, rows[0].Kind)
	require.Equal(t, "h1", rows[0].Host)
	require.Equal(t, RowSlot, rows[1].Kind)
	require.Equal(t, 1, rows[1].Slot.N)
	require.Equal(t, "active", rows[1].Slot.UnitState)
	require.Equal(t, "bobsled-h1-1", rows[1].RunnerName)
}

func TestBuildRows_HostCollapsedHasNoSlotRows(t *testing.T) {
	hosts := map[string]*poller.HostState{
		"h1": {Name: "h1", Slots: map[int]poller.SlotState{1: {N: 1}}},
	}
	rows := BuildRows(hosts, nil, map[string]bool{"h1": false})
	require.Len(t, rows, 1)
	require.Equal(t, RowHost, rows[0].Kind)
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/rows.go
package tui

import (
	"sort"

	"github.com/m-meyer2k/bobsled/internal/tui/poller"
)

type RowKind int

const (
	RowHost RowKind = iota
	RowSlot
)

type Row struct {
	Kind       RowKind
	Host       string
	HostState  *poller.HostState // nil if Kind==RowSlot
	Slot       poller.SlotState  // zero if Kind==RowHost
	RunnerName string            // matched runner from runners[repo], if found
}

// BuildRows flattens the tree into a vertical row list ready for rendering.
// Hosts are sorted alphabetically; slots numerically. Collapsed hosts skip
// their slot rows entirely.
func BuildRows(hosts map[string]*poller.HostState, runners map[string]*poller.RepoRunners, expanded map[string]bool) []Row {
	names := make([]string, 0, len(hosts))
	for k := range hosts {
		names = append(names, k)
	}
	sort.Strings(names)

	rows := make([]Row, 0, len(names)*4)
	for _, name := range names {
		h := hosts[name]
		rows = append(rows, Row{Kind: RowHost, Host: name, HostState: h})
		if !expanded[name] {
			continue
		}
		slotNums := make([]int, 0, len(h.Slots))
		for n := range h.Slots {
			slotNums = append(slotNums, n)
		}
		sort.Ints(slotNums)
		for _, n := range slotNums {
			slot := h.Slots[n]
			rows = append(rows, Row{
				Kind:       RowSlot,
				Host:       name,
				Slot:       slot,
				RunnerName: matchRunner(name, n, slot.Repo, runners),
			})
		}
	}
	return rows
}

func matchRunner(host string, slot int, repo string, runners map[string]*poller.RepoRunners) string {
	if runners == nil || runners[repo] == nil {
		return ""
	}
	want := "bobsled-" + host + "-" + itoa(slot)
	for _, r := range runners[repo].Runners {
		if r.Name == want {
			return r.Name
		}
	}
	return ""
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [16]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/rows.go internal/tui/rows_test.go
git commit -m "feat(tui): BuildRows turns the tree into a flat row list"
```

---

### Task 16: `internal/tui/layout.go` — lipgloss styling + View

**Files:**
- Create: `internal/tui/layout.go`
- Create: `internal/tui/layout_test.go`
- Modify: `internal/tui/model.go` (add `View()` method delegating to `layout.go`)

- [ ] **Step 1: Test (golden snapshot, ANSI-stripped)**

```go
// internal/tui/layout_test.go
package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

func TestRender_StableSnapshot(t *testing.T) {
	m := New(&inventory.Inventory{
		Hosts: map[string]inventory.Host{"h1": {SSH: "bobsled@h1", Capacity: 4}},
		Pools: []inventory.Pool{{Repo: "acme/foo", Count: 1, Spread: []string{"h1"}}},
	}, nil)
	m.Width, m.Height = 80, 24
	m.Hosts["bobsled@h1"] = &poller.HostState{
		Name: "h1", Reachable: true, Capacity: 4,
		Slots: map[int]poller.SlotState{1: {N: 1, UnitState: "active", Repo: "acme/foo"}},
	}
	m.Runners["acme/foo"] = &poller.RepoRunners{
		Runners: []ghapp.RunnerRef{{ID: 1, Name: "bobsled-h1-1"}},
	}
	m.Cursor = FirstCursor(m.Hosts, m.Expanded)

	out := stripANSI(m.View())
	require.Contains(t, out, "bobsled")
	require.Contains(t, out, "h1")
	require.Contains(t, out, "acme/foo")
	require.Contains(t, out, "active")
	require.True(t, strings.Contains(out, "j/k") || strings.Contains(out, "?"), "footer keybindings present")
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement `layout.go`**

```go
// internal/tui/layout.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	hostHeader     = lipgloss.NewStyle().Bold(true)
	hostUnreach    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	slotActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	slotActivating = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	slotFailed     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	cursorStyle    = lipgloss.NewStyle().Reverse(true)
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	flashErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func (m Model) View() string {
	if m.Width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderTree())
	b.WriteString("\n")
	b.WriteString(m.renderWorkload())
	b.WriteString("\n")
	b.WriteString(m.renderRecent())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m Model) renderHeader() string {
	totalSlots, busy := 0, 0
	for _, h := range m.Hosts {
		totalSlots += len(h.Slots)
	}
	for _, rep := range m.Runs {
		busy += len(rep.InProgress)
	}
	left := headerStyle.Render("bobsled top")
	right := fmt.Sprintf("fleet: %d hosts · %d slots · %d busy   ↻ %s",
		len(m.Hosts), totalSlots, busy, hostsInterval)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) renderTree() string {
	rows := BuildRows(m.Hosts, m.Runners, m.Expanded)
	var b strings.Builder
	for _, r := range rows {
		line := m.renderRow(r)
		if m.isCursorOn(r) {
			line = cursorStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m Model) renderRow(r Row) string {
	if r.Kind == RowHost {
		marker := "▼"
		if !m.Expanded[r.Host] {
			marker = "▶"
		}
		reach := "[ok]"
		if r.HostState != nil && !r.HostState.Reachable {
			reach = hostUnreach.Render("●UNREACHABLE")
		}
		used := 0
		cap := 0
		if r.HostState != nil {
			used = len(r.HostState.Slots)
			cap = r.HostState.Capacity
		}
		return hostHeader.Render(fmt.Sprintf("%s %-12s cap=%d used=%d   %s", marker, r.Host, cap, used, reach))
	}
	stateStyle := slotActive
	switch r.Slot.UnitState {
	case "activating":
		stateStyle = slotActivating
	case "failed":
		stateStyle = slotFailed
	}
	runner := r.RunnerName
	if runner == "" {
		runner = "—"
	}
	return fmt.Sprintf("    %d  %s  %-18s  %s", r.Slot.N, stateStyle.Render(r.Slot.UnitState), r.Slot.Repo, runner)
}

func (m Model) isCursorOn(r Row) bool {
	if r.Kind == RowHost {
		return m.Cursor.Host == r.Host && m.Cursor.Kind == CursorHost
	}
	return m.Cursor.Host == r.Host && m.Cursor.Kind == CursorSlot && m.Cursor.Slot == r.Slot.N
}

func (m Model) renderWorkload() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("─ Workload ─") + "\n")
	for repo, rs := range m.Runs {
		queued := len(rs.Queued)
		ip := len(rs.InProgress)
		fmt.Fprintf(&b, "  %-20s queued: %d   in-progress: %d\n", repo, queued, ip)
	}
	return b.String()
}

func (m Model) renderRecent() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("─ Recent ─") + "\n")
	for repo, rs := range m.Runs {
		shown := 0
		for _, r := range rs.Recent {
			if shown >= 3 {
				break
			}
			fmt.Fprintf(&b, "  #%d %-9s %-18s  %s\n", r.ID, r.Status, repo, r.Name)
			shown++
		}
	}
	return b.String()
}

func (m Model) renderFooter() string {
	help := footerStyle.Render(
		"j/k:nav  ⏎:expand/collapse  a:add slot  A:add host  d:drain  D:remove host  r:reset cache  l:logs  g:gc  R:refresh  ?:help  q:quit")
	if m.Flash != nil && time.Now().Before(m.Flash.Until) {
		style := footerStyle
		if m.Flash.IsError {
			style = flashErrStyle
		}
		return style.Render(m.Flash.Text) + "\n" + help
	}
	return help
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/layout.go internal/tui/layout_test.go
git commit -m "feat(tui): View() with header, tree, workload, recent, footer"
```

---

## Phase 8 — Modal + inline prompt

### Task 17: Confirm modal

**Files:**
- Create: `internal/tui/modal.go`
- Create: `internal/tui/modal_test.go`

- [ ] **Step 1: Test**

```go
// internal/tui/modal_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestModal_RejectsBlankConfirm(t *testing.T) {
	mod := NewConfirmModal("Drain host h1", "Continue?", nil)
	require.False(t, mod.ReadyToConfirm())
}

func TestModal_AcceptsYes(t *testing.T) {
	called := false
	mod := NewConfirmModal("Drain host h1", "Continue?", func() tea.Cmd { called = true; return nil })
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, mod.ReadyToConfirm())
	_ = mod.Confirm()
	require.True(t, called)
}

func TestModal_EscapeCancels(t *testing.T) {
	mod := NewConfirmModal("Drain host h1", "Continue?", nil)
	mod = mod.OnKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.True(t, mod.Cancelled)
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/modal.go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Modal struct {
	Title     string
	Body      string
	Input     string
	OnConfirm func() tea.Cmd
	Cancelled bool
}

func NewConfirmModal(title, body string, onConfirm func() tea.Cmd) Modal {
	return Modal{Title: title, Body: body, OnConfirm: onConfirm}
}

func (m Modal) OnKey(msg tea.KeyMsg) Modal {
	switch msg.Type {
	case tea.KeyEsc:
		m.Cancelled = true
		return m
	case tea.KeyBackspace:
		if len(m.Input) > 0 {
			m.Input = m.Input[:len(m.Input)-1]
		}
		return m
	case tea.KeyRunes:
		m.Input += string(msg.Runes)
		return m
	}
	return m
}

func (m Modal) ReadyToConfirm() bool {
	return strings.ToLower(strings.TrimSpace(m.Input)) == "yes"
}

func (m Modal) Confirm() tea.Cmd {
	if m.OnConfirm == nil || !m.ReadyToConfirm() {
		return nil
	}
	return m.OnConfirm()
}

// Render returns the styled modal contents, centered. Caller composes with View.
func (m Modal) Render(width int) string {
	box := "╭── " + m.Title + " ──╮\n│\n│  " + m.Body + "\n│\n│  Type 'yes' to confirm: " + m.Input + "_\n│\n│  [⏎ confirm]   [esc cancel]\n╰" + strings.Repeat("─", len(m.Title)+8) + "╯"
	return box
}

type InlinePrompt struct {
	Label    string
	Fields   []string
	Values   []string
	Focused  int
	OnSubmit func(values []string) tea.Cmd
}
```

- [ ] **Step 4: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/modal.go internal/tui/modal_test.go
git commit -m "feat(tui): Modal with typed-yes confirmation"
```

---

### Task 18: Wire modal into Update + key dispatch

**Files:**
- Modify: `internal/tui/model.go` (route KeyMsg to modal when present)
- Modify: `internal/tui/keys.go` (open modal on 'd' / 'D' / 'r' / 'g')
- Modify: `internal/tui/keys_test.go` (test the open + confirm path)

- [ ] **Step 1: Test**

Append to `internal/tui/keys_test.go`:

```go
func TestKey_D_OpensDrainSlotModal(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	mNew, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Modal)
	require.Contains(t, mm.Modal.Title, "Drain slot")
}

func TestKey_RoutedToModalWhenOpen(t *testing.T) {
	m := modelWithTwoHosts(t)
	m.Cursor = Cursor{Host: "h1", Kind: CursorSlot, Slot: 1}
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m, _ = updateOnce(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, m.Modal.ReadyToConfirm())
}

func updateOnce(m Model, msg tea.Msg) (Model, tea.Cmd) {
	r, c := m.Update(msg)
	return r.(Model), c
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Update keys.go**

Append cases to `handleKey` in `internal/tui/keys.go`:

```go
case 'd':
    title := "Drain slot"
    body := "Disable this slot. Existing jobs run to completion."
    if m.Cursor.Kind == CursorHost {
        title = "Drain host " + m.Cursor.Host
        body = "Disable every slot on this host."
    }
    mod := NewConfirmModal(title, body, nil) // Action wired in Phase 11
    m.Modal = &mod
    return m, nil
```

(`g` and `r` and `D` follow the same pattern in Phase 11; for now `d` is sufficient to exercise the modal plumbing.)

- [ ] **Step 4: Update Update to route to modal when open**

In `internal/tui/model.go`, top of `Update`:

```go
case tea.KeyMsg:
    if m.Modal != nil {
        next := m.Modal.OnKey(v)
        m.Modal = &next
        if next.Cancelled {
            m.Modal = nil
        }
        // On Enter when ReadyToConfirm, call Confirm and dismiss.
        if v.Type == tea.KeyEnter && next.ReadyToConfirm() {
            cmd := next.Confirm()
            m.Modal = nil
            return m, cmd
        }
        return m, nil
    }
    return m.handleKey(v)
```

- [ ] **Step 5: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/keys.go internal/tui/keys_test.go internal/tui/model.go
git commit -m "feat(tui): route keys to modal when open; 'd' opens drain modal"
```

---

## Phase 9 — Actions (drain, add, gc, cache reset)

### Task 19: `internal/tui/actions.go` — action dispatch

**Files:**
- Create: `internal/tui/actions.go`
- Create: `internal/tui/actions_test.go`

The actions are one-shot tea.Cmds. The simplest pattern: each action runs the same Go function the CLI uses (a thin wrapper that drives SSH or the GitHub API), and emits an `ActionResultMsg`.

- [ ] **Step 1: Test (model state after ActionResultMsg)**

```go
// internal/tui/actions_test.go
package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdate_ActionResultErrorFlashes(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(ActionResultMsg{Err: assertErr("boom")})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.True(t, mm.Flash.IsError)
	require.Contains(t, mm.Flash.Text, "boom")
}

func TestUpdate_ActionLogAppendsToBuffer(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(ActionLogMsg{Line: "hello"})
	mm := mNew.(Model)
	lines := mm.StatusLog.Lines()
	require.Equal(t, []string{"hello"}, lines)
}
```

- [ ] **Step 2: Run, expect fail**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/...
```

- [ ] **Step 3: Implement**

```go
// internal/tui/actions.go
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ActionLogMsg is emitted by long-running actions as they produce output.
type ActionLogMsg struct{ Line string }

// ActionResultMsg is emitted when an action finishes.
type ActionResultMsg struct {
	Description string
	Err         error
}

func (m Model) onActionLog(msg ActionLogMsg) (Model, tea.Cmd) {
	m.StatusLog.Push(msg.Line)
	return m, nil
}

func (m Model) onActionResult(msg ActionResultMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.Flash = &flash{Text: fmt.Sprintf("%s failed: %v", msg.Description, msg.Err), IsError: true, Until: time.Now().Add(5 * time.Second)}
	} else {
		m.Flash = &flash{Text: msg.Description + " ✓", Until: time.Now().Add(3 * time.Second)}
	}
	return m, nil
}
```

- [ ] **Step 4: Hook into Update**

Add cases at the end of the `switch v := msg.(type)` in `Update`:

```go
case ActionLogMsg:
    return m.onActionLog(v)
case ActionResultMsg:
    return m.onActionResult(v)
```

- [ ] **Step 5: Run, verify pass; commit**

```bash
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
git add internal/tui/actions.go internal/tui/actions_test.go internal/tui/model.go
git commit -m "feat(tui): action log + result message handling, footer flash"
```

---

### Task 20: Wire `d`/`D` to real drain action

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/keys_test.go`

The drain action shells out to `bobsled drain`. We invoke it via `exec.Command` so output streams; the existing drain subcommand is reused.

- [ ] **Step 1: Implement the action factory**

Append to `internal/tui/actions.go`:

```go
import (
	"bufio"
	"context"
	"os/exec"
)

// DrainSlotCmd runs `bobsled drain --host <h> --slot <n>` and streams output as
// ActionLogMsg lines, then emits ActionResultMsg.
func DrainSlotCmd(inventoryPath, host string, slot int) tea.Cmd {
	desc := fmt.Sprintf("drain %s slot %d", host, slot)
	return runAction(desc, inventoryPath, "drain", "--host", host, "--slot", fmt.Sprint(slot))
}

func DrainHostCmd(inventoryPath, host string) tea.Cmd {
	desc := fmt.Sprintf("drain host %s", host)
	return runAction(desc, inventoryPath, "drain", "--host", host)
}

func runAction(description, inventory string, args ...string) tea.Cmd {
	return func() tea.Msg {
		full := append([]string{"--inventory", inventory}, args...)
		cmd := exec.CommandContext(context.Background(), "./bin/bobsled", full...)
		out, err := cmd.CombinedOutput()
		// Note: we'd ideally stream line-by-line. For Task 20 we keep it simple
		// and emit the full output as one ActionLogMsg via the cmd chain.
		_ = out
		return ActionResultMsg{Description: description, Err: err}
	}
}

// LineStreamCmd is a helper for line-streaming actions (used by host add/remove
// in later tasks).
func LineStreamCmd(description string, build func() *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		cmd := build()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return ActionResultMsg{Description: description, Err: err}
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			return ActionResultMsg{Description: description, Err: err}
		}
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			// Without a channel back to the program we can't emit per-line.
			// Phase 12 (`tui` subcommand) wires this through tea.Program.Send().
			_ = s.Text()
		}
		return ActionResultMsg{Description: description, Err: cmd.Wait()}
	}
}
```

- [ ] **Step 2: Wire into key handler**

Modify the `case 'd'` block in `internal/tui/keys.go`:

```go
case 'd':
    var onConfirm func() tea.Cmd
    title := "Drain slot"
    body := "Disable this slot. Existing jobs run to completion."
    if m.Cursor.Kind == CursorHost {
        title = "Drain host " + m.Cursor.Host
        body = "Disable every slot on this host."
        host := m.Cursor.Host
        onConfirm = func() tea.Cmd {
            return DrainHostCmd(m.InventoryPath, host)
        }
    } else {
        host, slot := m.Cursor.Host, m.Cursor.Slot
        onConfirm = func() tea.Cmd {
            return DrainSlotCmd(m.InventoryPath, host, slot)
        }
    }
    mod := NewConfirmModal(title, body, onConfirm)
    m.Modal = &mod
    return m, nil
```

`InventoryPath` is a field we need on the Model — add it in `internal/tui/model.go`:

```go
type Model struct {
    ...
    InventoryPath string
}
```

And accept it in `New(...)`:

```go
func New(inv *inventory.Inventory, c *ghapp.Client, inventoryPath string) Model {
    ...
    InventoryPath: inventoryPath,
}
```

Update callers of `New` in tests to pass `"inventory.yaml"`.

- [ ] **Step 3: Build + test**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
PATH=/home/mike/.local/go/bin:$PATH go test ./internal/tui/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/actions.go internal/tui/keys.go internal/tui/model.go internal/tui/model_test.go internal/tui/keys_test.go
git commit -m "feat(tui): wire drain slot/host actions through the modal"
```

---

## Phase 10 — New CLI subcommands

### Task 21: `bobsled host add`

**Files:**
- Create: `internal/cli/host_add.go`
- Modify: `internal/cli/host_bootstrap.go` (register)

- [ ] **Step 1: Implement**

```go
// internal/cli/host_add.go
package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/m-meyer2k/bobsled/assets"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/spf13/cobra"
)

func newHostAddCmd() *cobra.Command {
	var (
		sshT, bootstrapSSH, repo, labels string
		appKey, mintBinary, imageDigest  string
		capacity, count                  int
		replace                          bool
		authorizedKeys                   string
	)
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "Bootstrap + install + add to inventory in one shot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			if _, exists := inv.Hosts[name]; exists && !replace {
				return fmt.Errorf("host %q already exists; use --replace to overwrite", name)
			}

			// 1) bootstrap
			runScript := exec.Command("ssh", bootstrapSSH, "bash -s")
			runScript.Stdin = bytes.NewReader(assets.BootstrapScript)
			runScript.Stdout = os.Stdout
			runScript.Stderr = os.Stderr
			if err := runScript.Run(); err != nil {
				return fmt.Errorf("bootstrap: %w", err)
			}
			keys, err := os.ReadFile(authorizedKeys)
			if err != nil {
				return fmt.Errorf("read authorized_keys: %w", err)
			}
			writeKeys := exec.Command("ssh", bootstrapSSH,
				"sudo install -m 0600 -o bobsled -g bobsled /dev/stdin /var/lib/bobsled/.ssh/authorized_keys")
			writeKeys.Stdin = bytes.NewReader(keys)
			writeKeys.Stdout = os.Stdout
			writeKeys.Stderr = os.Stderr
			if err := writeKeys.Run(); err != nil {
				return fmt.Errorf("install keys: %w", err)
			}

			// 2) install (delegate to existing host install code path)
			if imageDigest == "" {
				return fmt.Errorf("--image-digest required")
			}
			if err := installToHost(sshT, mintBinary, imageDigest, expandHome(firstNonEmpty(appKey, inv.GitHub.AppKey)), inv.GitHub.AppID, name); err != nil {
				return fmt.Errorf("install: %w", err)
			}

			// 3) update inventory.yaml
			newInv, err := inventory.AddHost(inv, name, inventory.Host{
				SSH: sshT, BootstrapSSH: bootstrapSSH, Capacity: capacity,
			})
			if err != nil {
				return err
			}
			if repo != "" && count > 0 {
				newInv, err = inventory.AdjustPool(newInv, repo, count, []string{name})
				if err != nil {
					return err
				}
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}

			// 4) apply if a pool was added/changed
			if repo != "" && count > 0 {
				return runApply(flagInventory)
			}
			fmt.Printf("host %s added\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&sshT, "ssh", "", "SSH target after bootstrap (e.g. bobsled@host) (required)")
	c.Flags().StringVar(&bootstrapSSH, "bootstrap-ssh", "", "admin SSH target for the one-time bootstrap (required)")
	c.Flags().IntVar(&capacity, "capacity", 4, "max slots on this host")
	c.Flags().StringVar(&repo, "repo", "", "(optional) repo to add a pool entry for")
	c.Flags().IntVar(&count, "count", 0, "(optional) initial pool count when --repo is set")
	c.Flags().StringVar(&labels, "labels", "", "comma-separated labels (default: self-hosted,linux,x64,bobsled,podman)")
	c.Flags().StringVar(&mintBinary, "mint-binary", "./bin/bobsled-mint", "local path to bobsled-mint")
	c.Flags().StringVar(&imageDigest, "image-digest", "", "wrapper image digest (sha256:...) (required)")
	c.Flags().StringVar(&appKey, "app-key", "", "override path to GitHub App private key")
	c.Flags().BoolVar(&replace, "replace", false, "allow replacing an existing host entry")
	c.Flags().StringVar(&authorizedKeys, "authorized-keys", os.ExpandEnv("$HOME/.ssh/id_ed25519.pub"), "operator pubkey to install on bobsled")
	_ = c.MarkFlagRequired("ssh")
	_ = c.MarkFlagRequired("bootstrap-ssh")
	_ = c.MarkFlagRequired("image-digest")
	return c
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
```

The helpers `installToHost` and `runApply` and `expandHome` are extracted from the existing `host_install.go` and `apply.go`. Add them as exported-internal functions in their respective files (or in a new `internal/cli/shared.go`). Implementation guidance:

```go
// internal/cli/shared.go
package cli

func installToHost(sshTarget, mintBinary, imageDigest, appKey string, appID int64, hostLabel string) error {
    // mirror the body of newHostInstallCmd.RunE here, taking exactly these
    // params; the existing newHostInstallCmd.RunE can be rewritten to call it.
    // (Keeps the host install code in one place.)
    // <copy from host_install.go>
}

func runApply(invPath string) error {
    // mirror NewApplyCmd's RunE: load inventory, allocate, applyHost per host.
}
```

Refactor `host_install.go` to call `installToHost` from `newHostInstallCmd.RunE`.

Refactor `apply.go` to call `runApply` from `newApplyCmd.RunE`.

- [ ] **Step 2: Register**

In `internal/cli/host_bootstrap.go`:

```go
func newHostCmd() *cobra.Command {
    host := &cobra.Command{Use: "host", Short: "Host lifecycle operations"}
    host.AddCommand(newHostBootstrapCmd())
    host.AddCommand(newHostInstallCmd())
    host.AddCommand(newHostUpgradeCmd())
    host.AddCommand(newHostRotateKeyCmd())
    host.AddCommand(newHostAddCmd())
    return host
}
```

- [ ] **Step 3: Build + smoke `--help`**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
PATH=/home/mike/.local/go/bin:$PATH go run ./cmd/bobsled host add --help | head -20
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/host_add.go internal/cli/host_bootstrap.go internal/cli/shared.go internal/cli/host_install.go internal/cli/apply.go
git commit -m "feat(cli): host add — bootstrap + install + inventory mutation"
```

---

### Task 22: `bobsled host remove`

**Files:**
- Create: `internal/cli/host_remove.go`
- Modify: `internal/cli/host_bootstrap.go` (register)

- [ ] **Step 1: Implement**

```go
// internal/cli/host_remove.go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/ssh"
	"github.com/spf13/cobra"
)

func newHostRemoveCmd() *cobra.Command {
	var (
		wipe          bool
		leaveRunners  bool
		timeout       time.Duration
	)
	c := &cobra.Command{
		Use:   "remove <name>",
		Short: "Drain a host, remove it from inventory, optionally wipe its user",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			host, ok := inv.Hosts[name]
			if !ok {
				return fmt.Errorf("host %q not in inventory", name)
			}

			// 1) drain all slots on this host
			s := &ssh.Client{Target: host.SSH}
			if _, err := s.Run("systemctl --user disable 'bobsled@*' 2>/dev/null || true"); err != nil {
				return fmt.Errorf("disable: %w", err)
			}
			// poll until inactive
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				out, _ := s.Run("systemctl --user is-active 'bobsled@*' 2>&1 || true")
				if !contains(out, "active") {
					break
				}
				time.Sleep(5 * time.Second)
			}

			// 2) gc orphans (delegate to gc subcommand for repos this host served)
			if !leaveRunners {
				_ = exec.Command("./bin/bobsled", "--inventory", flagInventory, "gc").Run()
			}

			// 3) wipe (optional)
			if wipe {
				wipeCmd := exec.Command("ssh", host.BootstrapSSH,
					"sudo systemctl stop user@$(id -u bobsled).service 2>/dev/null; "+
						"sudo userdel -r bobsled 2>/dev/null; "+
						"sudo rm -rf /var/lib/bobsled")
				wipeCmd.Stdout = os.Stdout
				wipeCmd.Stderr = os.Stderr
				if err := wipeCmd.Run(); err != nil {
					return fmt.Errorf("wipe: %w", err)
				}
			}

			// 4) inventory mutation
			newInv, err := inventory.RemoveHost(inv, name)
			if err != nil {
				return err
			}
			if err := inventory.Write(flagInventory, newInv); err != nil {
				return err
			}
			fmt.Printf("host %s removed\n", name)
			return nil
		},
	}
	c.Flags().BoolVar(&wipe, "wipe", false, "also userdel -r bobsled and rm -rf /var/lib/bobsled on the host")
	c.Flags().BoolVar(&leaveRunners, "leave-runners", false, "don't gc GitHub-side runners after drain")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "max time to wait for drain")
	return c
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Register**

Add to `newHostCmd`:

```go
host.AddCommand(newHostRemoveCmd())
```

- [ ] **Step 3: Build + smoke**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
PATH=/home/mike/.local/go/bin:$PATH go run ./cmd/bobsled host remove --help | head -15
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/host_remove.go internal/cli/host_bootstrap.go
git commit -m "feat(cli): host remove — drain, gc, optional wipe, inventory drop"
```

---

## Phase 11 — `bobsled tui` subcommand

### Task 23: Cobra wiring

**Files:**
- Create: `internal/cli/tui.go`
- Modify: `internal/cli/root.go` (register)

- [ ] **Step 1: Implement**

```go
// internal/cli/tui.go
package cli

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m-meyer2k/bobsled/internal/ghapp"
	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui"
	"github.com/spf13/cobra"
)

func newTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Live full-screen view of the fleet with keypress actions",
		RunE: func(_ *cobra.Command, _ []string) error {
			inv, err := inventory.Load(flagInventory)
			if err != nil {
				return err
			}
			c := &ghapp.Client{
				APIBase: "https://api.github.com",
				AppID:   inv.GitHub.AppID,
				KeyPath: expandHome(inv.GitHub.AppKey),
				HTTP:    &http.Client{Timeout: 30 * time.Second},
				Now:     time.Now,
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			tui.SetContext(ctx)
			m := tui.New(inv, c, flagInventory)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
```

Register in `internal/cli/root.go` `NewRoot()`:

```go
root.AddCommand(newTuiCmd())
```

- [ ] **Step 2: Build, smoke `--help`**

```bash
PATH=/home/mike/.local/go/bin:$PATH go build ./...
PATH=/home/mike/.local/go/bin:$PATH go run ./cmd/bobsled tui --help
```

- [ ] **Step 3: Commit**

```bash
git add internal/cli/tui.go internal/cli/root.go
git commit -m "feat(cli): `bobsled tui` subcommand starts the Bubbletea program"
```

---

## Phase 12 — End-to-end smoke

### Task 24: Manual smoke against the local fleet

- [ ] **Step 1: Rebuild**

```bash
cd /home/mike/Code/bobsled
PATH=/home/mike/.local/go/bin:$PATH make build
```

- [ ] **Step 2: Run the TUI**

```bash
./bin/bobsled --inventory inventory.yaml tui
```

Expected:
- Header bar with fleet summary
- Tree with `local` host expanded showing `1 active acme/foo bobsled-local-1`
- Workload section with `acme/bobsled-smoke queued: 0 in-progress: 0`
- Recent section with the prior `gh workflow run` results
- Footer keybindings line

- [ ] **Step 3: Try the nav + actions**

- Press `j` / `k`: cursor moves down/up.
- Press `⏎` on the host row: collapse / expand.
- Move to a slot row, press `d`: confirm-modal opens.
- Type `yes`, press `⏎`: drain runs; flash shows result.
- Press `q`: exits cleanly.

- [ ] **Step 4: Note any sharp edges; if blocked, capture details and report**

(no commit at this step — manual verification)

---

## Self-review checklist

- [ ] **Spec coverage:** Architecture (Tasks 12-13), tree layout (Tasks 15-16), modal (Tasks 17-18), keybindings (Task 14 + action keys merged into Phase 9), data model (Task 12), pollers with ETag (Tasks 7-10), SSH multiplexing (Task 6), inventory mutate (Task 2), atomic inventory write (Task 3), `host add` (Task 21), `host remove` (Task 22), TUI cobra wiring (Task 23), smoke (Task 24). Logs pager + the action keys `r`, `g`, `A`, `a`, `R`, `P`, `?`, `/`, `l` are **deferred** to a follow-up plan or to be added incrementally — the v1 plan ships with drain working end-to-end and the other keys as scaffolding to fill out.
- [ ] **No placeholders.**
- [ ] **Type consistency:** `poller.HostState`/`SlotState`, `poller.RepoRunners`/`RepoRuns`, `tui.Cursor`/`CursorKind`/`CursorHost`/`CursorSlot`, `tui.Row`/`RowKind`/`RowHost`/`RowSlot`, `tui.Modal`/`InlinePrompt`, message types `hostsTickMsg`/`runnersTickMsg`/`runsTickMsg`/`ActionLogMsg`/`ActionResultMsg`. Internal channel types `poller.HostsMsg`/`RunnersMsg`/`RunsMsg`.
- [ ] Every code step shows real code; every command step has an expected outcome; every task ends with a commit.

---

## Out of scope (deferred — call out in the spec's Open Questions)

- **Logs pager (`l` key)** — needs a viewport subview and SSH log streaming. Not in v1; press `l` shows a placeholder flash for now.
- **Inline prompts for `a` / `A`** — Phase 13 follow-up plan. The keys are no-ops in v1.
- **`r` (cache reset), `g` (gc), `D` (remove host)** — same modal pattern as `d`; wire them as a follow-up.
- **`R` (force refresh), `P` (pause), `/` (filter), `?` (help overlay)** — usability nice-to-haves; deferred.
- **Per-host backoff on consecutive failures** — pollers currently retry on the same fixed interval. Exponential backoff is a follow-up.

# bobsled tui — Design

**Date:** 2026-05-13
**Status:** Draft for review

## Purpose

A full-screen interactive TUI on top of the existing `bobsled` CLI: live fleet view, host + slot tree, GitHub-side runner state, workload (queued/in-progress runs), and direct keypress actions for add/drain/remove/cache-reset/gc — destructive operations gated by a confirm modal.

## Goals

- Single binary, same `bobsled` cobra surface: `bobsled tui` (or `bobsled top`) starts the program.
- Live updates with bounded cost — small fleets stay well under GitHub's 5000 req/hr rate limit via tiered polling cadences and ETag conditional requests.
- Action surface includes host lifecycle (add an entire host, remove an entire host) — not just slot-level operations.
- Lowercase keys for safe ops, uppercase for destructive ops; destructive ops require a typed `yes` in a modal.
- No new persistent state on the operator's machine beyond what the CLI already manages (`inventory.yaml`).

## Non-goals

- Mouse / scroll-wheel support (terminal users navigate by key).
- Multi-fleet view (separate inventories in one program).
- Persisting expand/collapse state across runs.
- Log aggregation beyond `journalctl` over SSH (no Loki / OTel / etc.).
- Replacing the existing CLI subcommands. They remain canonical; the TUI dispatches the same code paths.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│  bobsled tui  (cobra subcommand, runs locally)                           │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  Bubbletea Program                                              │    │
│  │  ┌─────────────────────────────────────────────────────────┐    │    │
│  │  │  Model (single struct, source of truth)                 │    │    │
│  │  │   - hosts: map[hostName]HostView                        │    │    │
│  │  │   - repos: map[ownerRepo]RepoView                       │    │    │
│  │  │   - lastErr per source, modal state, cursor, etc.       │    │    │
│  │  └────────────────────────▲────────────────────────────────┘    │    │
│  │                           │ tea.Msg                              │    │
│  │  ┌────────────────────────┴────────────────────────────────┐    │    │
│  │  │  Update(msg) → (Model, Cmd)                             │    │    │
│  │  └────────────────────────┬────────────────────────────────┘    │    │
│  │                           ▼ render                               │    │
│  │  ┌──────────────────────────────────────────────────────────┐   │    │
│  │  │  View() string  →  rendered TTY frame                    │   │    │
│  │  └──────────────────────────────────────────────────────────┘   │    │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│         ▲                  ▲                  ▲                          │
│         │ HostsMsg         │ RunnersMsg       │ RunsMsg                  │
│         │ every 2s         │ every 3s ETag    │ every 15s ETag           │
│  ┌──────┴─────┐  ┌─────────┴────────┐  ┌──────┴──────────┐               │
│  │ host poller│  │ runners poller   │  │ runs poller     │               │
│  └──────┬─────┘  └─────────┬────────┘  └──────┬──────────┘               │
└─────────┼──────────────────┼──────────────────┼──────────────────────────┘
          ▼                  ▼                  ▼
   bobsled@hosts        api.github.com    api.github.com
   systemctl --user     /repos/.../runners /repos/.../actions/runs
   cat state.yaml
```

**Key properties:**
- Pollers are pure goroutines started by Bubbletea `Cmd`s at program init. Each runs forever and pushes `tea.Msg`s onto Bubbletea's channel.
- The model is single-writer (only `Update` mutates). Pollers never touch the model directly — they emit messages.
- Actions are one-shot `tea.Cmd`s that run in goroutines and produce an `ActionResult` message when done. While in-flight, the affected row shows a spinner suffix.
- ETag caching: a "no change since last poll" round-trip is ~30 bytes and **does not** count against the GitHub App's 5000 req/hr quota.

## Framework

- `github.com/charmbracelet/bubbletea` — the model/update/view loop.
- `github.com/charmbracelet/lipgloss` — styled rendering (colors, borders, padding).
- `github.com/charmbracelet/bubbles/viewport` — for the logs pager.
- `github.com/charmbracelet/bubbles/textinput` — for inline prompts in modals.

No additional GitHub SDK. The existing `internal/ghapp` package gains ETag-aware variants of `ListRepoRunners` and adds a new `ListWorkflowRuns(repo, status)` method.

## Layout

Tree view with hosts as parents, slots as children. Two side panels surface GitHub-side data without crowding the main tree.

```
┌─ bobsled top ──────────  fleet: 2 hosts · 6 slots · 4 busy  ───  ↻ 2s ─┐
│                                                                       │
│  ▼ local    cap=4 used=2 ssh=bobsled@localhost          [ok]          │
│      1  active   acme/foo   bobsled-local-1   busy #45  (1m12s)       │
│    ▸ 2  active   acme/foo   bobsled-local-2   idle                    │
│                                                                       │
│  ▼ runner-2 cap=4 used=3 ssh=bobsled@runner-2.lan       [ok]          │
│      1  active   acme/bar   bobsled-runner-2-1  busy #61 (4s)         │
│      2  restart  acme/bar   (minting…)          —                     │
│      3  active   acme/bar   bobsled-runner-2-3  idle                  │
│                                                                       │
│  ▽ runner-3   ssh=bobsled@runner-3.lan   ●UNREACHABLE  last seen 2m   │
│                                                                       │
├──────────────────────── Workload ─────────────────────────────────────┤
│  acme/foo    queued: 0    in-progress: 1                              │
│  acme/bar    queued: 3 ⚠  in-progress: 2                              │
│                                                                       │
├──────────────────────── Recent (last 10) ─────────────────────────────┤
│  #45 ✓ 12s   acme/foo  hello             bobsled-local-1   8s         │
│  #61 ▸ 4s    acme/bar  build-and-test    bobsled-runner-2-1  …        │
│  #44 ✓ 1m    acme/foo  hello             bobsled-local-2   6s         │
│                                                                       │
├───────────────────────────────────────────────────────────────────────┤
│ j/k:nav  ⏎:expand/collapse  a:add slot  A:add host  d:drain slot      │
│ D:remove host  r:reset cache  l:logs  g:gc  R:refresh  ?:help  q:quit │
└───────────────────────────────────────────────────────────────────────┘
```

**Three regions:**

1. **Fleet tree** (top, ~60% of height). Hosts collapse/expand with `⏎` or `→/←`. Cursor (`▸`) sits on a slot or a host header. Status badges per host: `[ok]`, `[degraded]` (some slots failing), `●UNREACHABLE` (SSH timed out).
2. **Workload** (middle, ~20%). Per-repo queue + in-progress count. ⚠ on a queue that exceeds the pool size for that repo (under-provisioned signal).
3. **Recent runs** (bottom, ~20%). Tail of the last ~10 workflow runs across all watched repos with the runner each ran on. Live-updated.

**Footer:** keybindings, always visible. `?` opens a fuller help overlay.

**Modals.** Destructive actions open a small centered modal that requires typing `yes` to confirm:

```
                ╭── Drain host runner-2 ─────────────╮
                │                                    │
                │  This disables 3 slots and waits   │
                │  for in-flight jobs to finish.     │
                │                                    │
                │  Type 'yes' to confirm: _          │
                │                                    │
                │  [⏎ confirm]   [esc cancel]        │
                ╰────────────────────────────────────╯
```

Action progress shows in a small status line at the bottom right (`draining runner-2: 2/3 slots stopped…`).

## Keybindings

Two principles: **lowercase = safe, uppercase = needs typed confirmation**. Context-sensitive: a key's meaning depends on whether the cursor is on a host header or a slot row.

| Key | Cursor on host | Cursor on slot | Modal? |
|---|---|---|---|
| `j`/`↓`, `k`/`↑` | move down/up | move down/up | no |
| `⏎`, `→`/`←` | expand/collapse host | open slot detail (logs, last 5 runs) | no |
| `a` | add a slot to this host | add a slot to this slot's host | inline prompt |
| `A` | **add a new host** (prompts SSH target, capacity, repo) | same | inline prompt |
| `d` | drain *all* slots on this host | drain *this* slot | "type yes" |
| `D` | **remove host entirely** (drain → optional userdel) | n/a | "type yes" + checkbox for `userdel -r` |
| `r` | reset cache for all slots on this host | reset cache for this (slot, repo) | "type yes" |
| `g` | run `bobsled gc` for repos served by this host | same | "type yes", `--dry-run` first by default |
| `l` | tail journalctl for the host's user systemd | tail journalctl for this slot's unit | no — opens pager |
| `R` | force-refresh all data sources now | same | no |
| `P` | pause / unpause polling and freeze the view | same | no |
| `/` | filter (live, fuzzy) — narrows tree to matching hosts/slots/repos | same | no |
| `?` | help overlay | same | overlay |
| `q`, `Ctrl-C` | quit | same | no |

**Inline prompts** (a one-line input at the bottom, like vim's `:` line):

```
add slot to runner-2 → repo: acme/_foo___  count delta: +1  [⏎ go] [esc]
```

```
add new host →  ssh: bobsled@runner-3.lan    bootstrap_ssh: mike@runner-3.lan
                capacity: 4    repo: acme/foo   count: 2     [⏎ go] [esc]
```

**Async action lifecycle:** when you confirm an action, the row gains a small spinner suffix (`(draining…)`) and the bottom-right shows a one-line live log. When done, spinner clears and the next poll picks up the new steady state. Errors bubble as a flash message on the footer (red, 5s dismiss).

## Data model

```go
// internal/tui/model.go
type Model struct {
    inv  *inventory.Inventory      // re-read on `R` or inventory file change

    // Snapshots, each updated by its own poller message.
    hosts   map[string]*HostState   // SSH probe state
    runners map[string]*RepoRunners // per repo: list of registered runners
    runs    map[string]*RepoRuns    // per repo: recent + in-progress

    // UI state
    cursor       Cursor          // tree path: host name, slot int, kind
    expanded     map[string]bool // hostName → expanded?
    modal        *Modal          // nil when no modal
    inline       *InlinePrompt   // nil when no prompt
    statusLog    *RingBuffer     // last 5 lines of action progress
    flash        *Flash          // transient error or success
    width, height int
    paused       bool            // P pauses polling
}

type HostState struct {
    Name       string
    Slots      map[int]SlotState
    Capacity   int
    Reachable  bool
    LastError  string
    LastUpdate time.Time
}

type SlotState struct {
    N         int
    UnitState string // active / activating / failed / inactive
    Repo      string // from state.yaml
    Container string // bobsled-1 if present
    StartedAt time.Time
}

type RepoRunners struct {
    Runners []ghapp.RunnerRef
    ETag    string
    Updated time.Time
}

type RepoRuns struct {
    Queued     []ghapp.WorkflowRun
    InProgress []ghapp.WorkflowRun
    Recent     []ghapp.WorkflowRun // last 10 across statuses
    ETag       string
    Updated    time.Time
}
```

## Pollers

Three independent goroutines under `internal/tui/poller`. Each emits one of:

```go
type HostsMsg   struct { Host string; State *HostState; Err error }
type RunnersMsg struct { Repo string; State *RepoRunners; Err error }
type RunsMsg    struct { Repo string; State *RepoRuns;   Err error }
```

| Poller | Interval | Per-tick cost | Conditional? |
|---|---|---|---|
| `hosts` | 2s | 1 SSH per host (multiplexed via `ControlMaster`) | Compare-and-swap: if state unchanged, skip the `Msg` |
| `runners` | 3s | 1 GitHub API per repo | `If-None-Match: <etag>` → 304 if unchanged, no quota burn |
| `runs` | 15s | 2 GitHub APIs per repo (queued + in_progress) + 1 for recent | Same ETag |

**Cost math** (for the fleet shape implied by the inventory — 1–3 hosts, ≤ 5 repos): peak ≈ 5 repos × (3s/3s + 1.5s/15s) ≈ ~2 req/s ≈ 7200 req/hr nominal, but with ETags most ticks return 304 and don't count against quota. Real consumed quota is closer to ~500 req/hr per repo under heavy change, much less when idle.

**SSH multiplexing:** the TUI sets `ssh -o ControlMaster=auto -o ControlPath=/tmp/bobsled-tui-%C -o ControlPersist=60` so the per-host poll opens *one* TCP/TLS+auth session and reuses it. First poll ~500 ms; subsequent ~30 ms.

**Pause-on-modal:** while a modal or inline prompt is open, `View()` uses a frozen snapshot so the user doesn't see flicker mid-typing. Pollers keep running; the model updates but the renderer ignores the new data until the modal closes.

## Action dispatch

Actions reuse the same Go functions the CLI subcommands wrap (apply, scale, drain, cache reset, gc, host add/remove). Output is line-streamed back into `statusLog` so the user sees what's happening.

The flow:
1. Keypress triggers a modal or inline prompt.
2. On confirm, the model enters an `Acting` state for the targeted row (spinner suffix in the View).
3. A `tea.Cmd` runs the action in a goroutine. As it produces output lines, it sends `ActionLogMsg{line}` which the model appends to `statusLog`.
4. When the action returns, an `ActionResultMsg{err}` is sent. The model clears the spinner; on error, flashes the bottom bar.
5. The next poll picks up the new steady state and the row updates naturally.

## New CLI subcommands

Two additions to the existing CLI surface — both useful standalone, both invoked by the TUI:

**`bobsled host add <name>`** — fold bootstrap + install + inventory mutation + apply into one shot.

```
bobsled host add <name>
    --ssh bobsled@host
    --bootstrap-ssh user@host
    --capacity N
    [--repo owner/name --count N --labels self-hosted,linux,x64,bobsled,podman]
    [--mint-binary ./bin/bobsled-mint]
    [--image-digest sha256:...]
    [--app-key path]
    [--replace]
```

Flow:
1. Read `inventory.yaml`. If `<name>` already exists, error unless `--replace`.
2. Run `assets/bootstrap.sh` over `--bootstrap-ssh`.
3. Push operator's pubkey into `~bobsled/.ssh/authorized_keys`.
4. Run `host install` against `--ssh` (binary, unit, config, key, digest).
5. Add the host block to `inventory.yaml` (atomic write: temp + rename).
6. If `--repo`/`--count` supplied, add/extend the matching pool and run `apply`.

**`bobsled host remove <name>`** — the inverse.

```
bobsled host remove <name>
    [--wipe]           # also userdel -r bobsled on the host
    [--leave-runners]  # don't gc GitHub-side runners; default cleans them
    [--timeout 30m]    # max time to wait for in-flight jobs
```

Flow:
1. Drain every slot on `<name>` (`systemctl --user disable` + wait until inactive).
2. Run `bobsled gc` scoped to repos that lost their last bobsled host.
3. If `--wipe`: `ssh bootstrap_ssh "sudo userdel -r bobsled && sudo rm -rf /var/lib/bobsled"`. Otherwise leave the user/data so re-adding is fast.
4. Remove the host block from `inventory.yaml` (atomic write). If a pool's `spread:` is left empty, prune the pool too.

**Inventory mutations** live in a new helper at `internal/inventory/mutate.go`:

```go
func AddHost(inv *Inventory, name string, h Host) (*Inventory, error)
func RemoveHost(inv *Inventory, name string) (*Inventory, error)
func AdjustPool(inv *Inventory, repo string, delta int, hosts []string) (*Inventory, error)
```

All take a parsed `Inventory`, return a new `Inventory`, and the caller is responsible for writing it. Tests assert round-trip equivalence (parse → mutate → marshal → re-parse → matches expected).

**Atomic inventory write:** same pattern as state.yaml — `os.CreateTemp` next to `inventory.yaml` + `os.Rename`. Concurrent `host add`s could race; we accept that and document "don't run two `host add`s at the same time." Inventory operations are rare enough that a flock would be YAGNI.

## File structure

```
cmd/bobsled/main.go                       # unchanged
internal/cli/
    tui.go                                # `bobsled tui` cobra wiring
    host_add.go                           # NEW
    host_remove.go                        # NEW
internal/inventory/
    mutate.go                             # NEW: AddHost, RemoveHost, AdjustPool
    mutate_test.go
internal/ghapp/
    runs.go                               # NEW: ListWorkflowRuns(repo, status)
    runs_test.go
    runners.go                            # MODIFIED: add ETag-aware variant
internal/tui/
    model.go                              # Model + tea.Update + tea.View
    model_test.go
    keys.go                               # key map, action dispatch
    layout.go                             # tree/workload/recent rendering
    layout_test.go                        # golden-file rendering tests
    modal.go                              # confirm modal + inline prompt
    poller/
        hosts.go
        runners.go
        runs.go
        poller_test.go
    pager.go                              # journalctl pager subview
```

## Error handling

**Per-source error surfacing.** Each poller has its own error slot in the model. Host header badges:

- `[ok]` — last poll succeeded
- `[stale 12s]` — last poll succeeded but the next one is overdue
- `●UNREACHABLE` — last 3 polls failed; the offending error preview is visible in the host detail (`⏎` to expand)

**GitHub-side failures** (auth, rate-limit, 5xx) appear as a flash bar at the top: `github: 429 Too Many Requests — backing off 60s`. The poller doubles its interval on consecutive failures (3s → 6s → 12s, cap 60s) and resets on the first success.

**Action errors.** If `host add` fails halfway (bootstrap succeeded, install failed), the TUI shows the partial-success state plus the actual error in the status log. No automatic rollback — the operator decides whether to retry, `host remove --wipe`, or fix manually. The CLI command itself prints exactly what step failed at exit.

## Logs view

Pressing `l` opens a pager-style panel that shells out to:

- For a slot: `ssh bobsled@host journalctl --user -u bobsled@N -f -n 200`
- For a host: same without `-u <unit>`, filtered to bobsled units

The pager honors `j/k`, `g/G`, `/` for in-pager search, `q` to return. Implementation: bubbletea's `viewport` component fed by a streaming SSH command.

## Testing

| Layer | How |
|---|---|
| Model `Update()` | Pure-function tests: synthesize each `tea.Msg`, assert state transitions. Cover all key bindings and message types. |
| Pollers | Mock the SSH client and the GitHub HTTP client (httptest). Assert cadence (fake clock), ETag handling, backoff on failure. |
| Inventory mutations | Round-trip property tests (`parse → AddHost → marshal → parse → match`). |
| New `host add` / `host remove` | Table-driven unit tests for the inventory mutation; integration via a smoke-style test against the actual local fleet. |
| Rendering (`View()`) | Golden-file tests at 80×24 and 120×40. Strip ANSI, compare. |

## Open questions / deferred

- **Mouse support.** Not in v1.
- **Multi-fleet view.** Not in v1.
- **Persistent expand/collapse state.** Not in v1.
- **Beyond journalctl.** No Loki/OTel integration in v1.
- **Inventory hot-reload.** v1 re-reads inventory only on `R` (force refresh). A file-watcher (`fsnotify`) is a possible follow-up.
- **Authoritative state on lossy networks.** If a host stays `UNREACHABLE` for long, the TUI eventually marks slots as stale. We don't try to "deduce" actual state — operator inspects.

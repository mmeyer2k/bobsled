# bobsled — Design

**Date:** 2026-05-13
**Status:** Draft for review

## Purpose

Orchestrate a small fleet (1–3 hosts, 5–20 runners total) of self-hosted GitHub Actions runners with strong isolation, podman-in-podman support, bulk enrollment, and multi-host coordination. Operated from a single Go CLI over SSH. The entire on-host stack runs unprivileged: rootless podman driven by user-level systemd, no root daemon.

## Goals

- Ephemeral runners: each runner consumes exactly one job, then is replaced by a fresh container.
- Strong isolation: workflow code runs inside a rootless podman container (the "wrapper"), with inner podman for any `docker`/`podman` commands the workflow issues.
- Bulk enrollment via a declarative `inventory.yaml`.
- Multi-host: same machinery on every host, no host-to-host traffic.
- Self-healing: hosts continue replacing consumed runners even when the operator's machine is offline.
- Per-slot persistent cache, including warm inner-podman layer cache.

## Non-goals

- Autoscaling. Pool sizes are fixed by inventory; operator changes inventory and runs `apply` to resize.
- Kubernetes / ARC. Out of scope at this scale.
- Untrusted PR workflows from forks. Threat model assumes workflows are written by trusted committers. (Wrapper container gives one extra kernel-namespace boundary anyway.)
- Cross-host cache sharing. Per-slot caches only. A pull-through registry mirror can be bolted on later if needed.

## Architecture

```
                  ┌──────────────────────────────────────────────┐
                  │ Operator's machine                           │
                  │   bobsled (Go CLI)                       │
                  │   inventory.yaml + GitHub App private key    │
                  └────────┬─────────────────────────────────────┘
                           │  SSH (bootstrap, scale, drain, apply,
                           │       ls, upgrade, gc, cache reset)
                           ▼
   ┌───────────────────────────────────────────────────────────────┐
   │ Host h1 (Linux, podman, systemd, user `bobsled` w/ linger) │
   │                                                               │
   │   user-level systemd (systemctl --user, run as `bobsled`): │
   │                                                               │
   │   bobsled@{1..N}.service (template, Restart=always)        │
   │     ExecStartPre: %h/bin/bobsled-mint                      │
   │                     --instance %i --output %t/.../jit.json    │
   │     ExecStart:    podman run --rm --userns=keep-id            │
   │                     --read-only --security-opt=...            │
   │                     -v %t/bobsled/%i:/jit:ro               │
   │                     -v ~/.cache/bobsled/slots/%i/current:/cache │
   │                     bobsled:<digest>                       │
   │     ExecStopPost: rm -rf %t/bobsled/%i                     │
   │                                                               │
   │   Wrapper container entrypoint:                               │
   │     1. read /jit/jit.json                                     │
   │     2. exec ./run.sh --jitconfig "$cfg"                       │
   │        (actions/runner picks up exactly one job, then exits)  │
   │                                                               │
   │   Inner rootless podman (PiP) inside wrapper:                 │
   │     storage rooted at /cache/podman-storage                   │
   │     fuse-overlayfs + user namespaces                          │
   └───────────────────────────────────────────────────────────────┘

   Hosts h2, h3, … identical. No host↔host traffic.
```

Each host is autonomous. The CLI is the only thing that knows the full fleet. The CLI SSHes directly to `bobsled@host` (service-account pattern; operator's keys go in `~bobsled/.ssh/authorized_keys`) and drives `systemctl --user`.

`%t` is the user instance's `$XDG_RUNTIME_DIR`, i.e. `/run/user/<uid>/`. `~` refers to `bobsled`'s home (`/var/lib/bobsled`).

## Host layout

All paths below live in the `bobsled` user's home and runtime directory. No system-level config, no root-owned files at runtime.

```
/var/lib/bobsled/                          # bobsled's $HOME
  ├── .ssh/authorized_keys                    # operator SSH keys (service-account)
  ├── config.yaml                             # static config (app_id, key path, GH API base)
  ├── app-key.pem                             # GitHub App private key, 0600
  ├── state.yaml                              # single source of truth for this host
  ├── image-digest.env                        # BOBSLED_IMAGE_DIGEST=sha256:...
  ├── bin/bobsled-mint                     # one-shot Go binary
  ├── .config/systemd/user/
  │     └── bobsled@.service               # user-level template unit
  └── .cache/bobsled/slots/<N>/            # per-(slot, repo) persistent cache, 0700
        ├── <owner>--<repo>/                  # one dir per repo this slot has served
        │     └── podman-storage/             #   inner podman graph root for (slot, repo)
        └── current → <owner>--<repo>/        # symlink to the active repo; updated by mint

/run/user/<uid>/bobsled/<N>/               # ephemeral per-job state (JIT config),
                                              # created by RuntimeDirectory= in unit
```

A `bobsled` system user owns everything. `loginctl enable-linger bobsled` keeps its user-level systemd instance running across logouts and reboots. No daemon written by us. No long-running broker.

### Host requirements

Created/configured by `bobsled host bootstrap`:

- Linux with systemd ≥ 240 and `podman` ≥ 4.x installed.
- A `bobsled` system user, home `/var/lib/bobsled`, with a real login shell (needed for `systemctl --user` over SSH).
- Subuid/subgid ranges allocated for `bobsled` in `/etc/subuid` and `/etc/subgid` (so the outer rootless podman can user-namespace its containers). The wrapper image ships with its own internal subuid/subgid configuration so that the inner podman can user-namespace its containers in turn.
- `fuse-overlayfs` and `slirp4netns` packages installed.
- `loginctl enable-linger bobsled` so the user-level systemd instance persists across logouts and reboots.
- Operator SSH keys appended to `~bobsled/.ssh/authorized_keys`.

These are the only steps that require root on the host. Once bootstrap is done, all subsequent CLI operations connect as `bobsled` and run unprivileged.

## Components

### 1. Wrapper OCI image (`bobsled:<digest>`)

Built from a `Containerfile` checked into the repo.

- Base: `quay.io/podman/stable` (provides rootless podman + fuse-overlayfs preconfigured).
- Adds: pinned `actions/runner` tarball, curl, jq, git.
- Runs as a non-root user (UID 1000) inside. Subuid/subgid ranges configured so inner podman can do its own user-namespacing.
- Entrypoint `/entrypoint.sh`:
  - reads `/jit/jit.json`,
  - extracts `encoded_jit_config`,
  - exec's `./run.sh --jitconfig "$cfg"`.
- No SSH server, no extra services. Read-only root filesystem (writable layers go to `/cache` and `/tmp` via tmpfs).

Image is built by the operator (`bobsled image build`) and shipped to hosts either via a registry the hosts can pull from or as an OCI archive `scp`'d to each host. The pinned digest is written to `/var/lib/bobsled/image-digest.env` (as `BOBSLED_IMAGE_DIGEST=sha256:...`) so the systemd unit always uses the exact image the operator chose. `host upgrade` rewrites this file atomically; running units pick up the new digest at their next natural restart.

### 2. `bobsled-mint` — one-shot Go binary, ~200 LOC

Invoked by `ExecStartPre` on every runner restart. Stateless. Lives at `~bobsled/bin/bobsled-mint`.

Flow:
1. Read `~bobsled/config.yaml` (app_id, app_key path, GitHub API base).
2. Read `~bobsled/state.yaml`, look up the entry for `--instance <N>` to find the target repo and label set.
3. Ensure the per-(slot, repo) cache exists and point the symlink at it: `mkdir -p ~/.cache/bobsled/slots/<N>/<owner>--<repo>/` then atomically replace `~/.cache/bobsled/slots/<N>/current` to point at it (write a temp symlink and `rename(2)` it into place — atomic on the same filesystem).
4. JWT-sign with the App private key; fetch an installation token for the repo's installation.
5. POST `/repos/{owner}/{repo}/actions/runners/generate-jitconfig` with:
   - **name**: `bobsled-<host>-<slot>` (e.g. `bobsled-h1-7`) — encodes provenance so `bobsled gc` can identify orphans.
   - **labels**: from the repo's entry in state.yaml — by convention `[bobsled, podman, …]`. The auto-labels (`self-hosted`, `linux`, `x64`) are added by the actions/runner itself and must not be passed here.
   - **runner_group_id**: `1` (default group; this design is repo-scoped, so runner groups aren't used).
6. Write the response JSON to `--output` (a path under `/run/user/<uid>/bobsled/<N>/`).
7. Exit 0. Non-zero on any failure — systemd treats it as a unit start failure and restarts per its policy.

Idempotent in the sense that a failed mint can simply be retried; JIT configs are single-use, an unused one expires harmlessly.

### 3. `bobsled@.service` — user-level systemd template unit

Installed at `~bobsled/.config/systemd/user/bobsled@.service`. Driven via `systemctl --user`.

```ini
[Unit]
Description=bobsled GitHub Actions runner slot %i
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
EnvironmentFile=%h/image-digest.env
RuntimeDirectory=bobsled/%i
RuntimeDirectoryMode=0700
ExecStartPre=%h/bin/bobsled-mint \
             --instance %i \
             --output %t/bobsled/%i/jit.json
ExecStart=/usr/bin/podman run --rm \
             --name=bobsled-%i \
             --userns=keep-id \
             --read-only \
             --security-opt=no-new-privileges \
             --cap-drop=ALL \
             --tmpfs=/tmp \
             -v %t/bobsled/%i:/jit:ro \
             -v %h/.cache/bobsled/slots/%i/current:/cache \
             bobsled:${BOBSLED_IMAGE_DIGEST}
ExecStopPost=/bin/rm -rf %t/bobsled/%i
Restart=always
RestartSec=2s
StartLimitBurst=10
StartLimitIntervalSec=120

[Install]
WantedBy=default.target
```

`%h` is `bobsled`'s home, `%t` is its `$XDG_RUNTIME_DIR` (`/run/user/<uid>/`). No `User=` directive — user-level units run as the owning user by definition. The image digest is sourced from `image-digest.env` (`BOBSLED_IMAGE_DIGEST=sha256:...`). `Restart=always` is the engine of self-healing: when actions/runner finishes its one job and exits, the unit comes back up, mints a new JIT config, runs a fresh container.

### 4. `bobsled` CLI

A single Go binary on the operator's machine. Connects to each host as `ssh bobsled@<host>` and drives `systemctl --user`. Subcommands:

| Subcommand | Purpose |
|---|---|
| `host bootstrap <host>` | One-shot privileged step. Connects as the bootstrap-time admin user, creates the `bobsled` user, allocates subuid/subgid, installs packages, enables linger, seeds `~bobsled/.ssh/authorized_keys` with operator keys. All subsequent CLI ops connect as `bobsled`. |
| `host install <host>` | Push mint binary, wrapper image, systemd unit, and `config.yaml`. `systemctl --user daemon-reload`. Idempotent. |
| `host upgrade <host>` | Replace mint binary and/or wrapper image, rewrite `image-digest.env`, `daemon-reload`. Running units pick up the new digest on their next natural restart. |
| `host rotate-key <host>` | Push a new App private key, atomically replace `app-key.pem`. |
| `scale --host h1 --repo acme/foo --count N [--labels ...]` | Imperative: update `state.yaml` on h1, `systemctl --user enable --now bobsled@K…K+N`. |
| `drain --host h1 [--repo ...] [--slot N]` | `systemctl --user disable` matching units; existing jobs run to completion; units do not restart. |
| `apply -f inventory.yaml` | Declarative: reconcile every host's `state.yaml` to match the inventory. The bulk-enrollment path. |
| `ls [--host ...] [--repo ...]` | Show each slot: host, repo, systemd state, GitHub status (idle/busy/offline). |
| `gc [--host ...]` | List runners on the GitHub side per repo, cross-reference inventory, delete orphans via API. |
| `cache reset --host h1 [--slot N] [--repo acme/foo]` | After stopping the targeted unit(s): wipe a specific repo's cache for a slot, all repos' caches for a slot, or every cache on the host. Re-enable the unit(s) afterward. |
| `cache gc --host h1` | Remove `slots/<N>/<repo>/` dirs that are not the current repo for that slot. Reclaim space without touching live caches. |
| `image build` / `image push` | Build the wrapper image, push to a registry or export an OCI archive. |

The CLI is stateless. The only operator-side state is `inventory.yaml` and the App key. Two distinct SSH targets per host: the bootstrap-time admin (only used by `host bootstrap`) and `bobsled@host` (everything else).

### 5. `inventory.yaml` — operator's single source of truth

```yaml
github:
  app_id: 123456
  app_key: ~/.bobsled/app-key.pem   # local path; CLI uploads to hosts

hosts:
  h1:
    ssh: bobsled@runner-1.lan          # used by all ops once bootstrapped
    bootstrap_ssh: mike@runner-1.lan      # only used by `host bootstrap`
    capacity: 8
  h2:
    ssh: bobsled@runner-2.lan
    bootstrap_ssh: mike@runner-2.lan
    capacity: 4

pools:
  - repo: acme/foo
    count: 6
    labels: [bobsled, podman]
    spread: [h1, h2]
  - repo: acme/bar
    count: 2
    labels: [bobsled, podman, bar-secrets]
    spread: [h1]
```

`bobsled apply -f inventory.yaml` diffs each host's current state.yaml against the inventory's allocation and converges. Allocation across `spread` hosts is deterministic (sorted by host name, fill to capacity, then next host).

### 6. `state.yaml` — per-host state file

`/var/lib/bobsled/state.yaml` on each host:

```yaml
repos:
  acme/foo:
    labels: [bobsled, podman]
  acme/bar:
    labels: [bobsled, podman, bar-secrets]
instances:
  1: { repo: acme/foo }
  2: { repo: acme/foo }
  3: { repo: acme/foo }
  7: { repo: acme/bar }
```

- Updated atomically by the CLI (write temp, rename).
- Read by `bobsled-mint` on each runner start.
- Adding instance 8 = add a line, enable `bobsled@8`.
- Removing instance 3 = disable `bobsled@3`, drop the line.
- Changing a repo's labels = edit the `repos` block once, all instances pick up the change on their next restart.

## Labels & runner names

GitHub auto-attaches three labels to every self-hosted runner; the orchestrator must **not** pass them at registration:

- `self-hosted`
- OS: `linux` (or `windows` / `macOS`)
- Arch: `x64` / `arm64` / `arm`

Bobsled adds two custom labels by convention:

- **`bobsled`** — fleet identifier. Every runner this orchestrator manages carries it. Workflows opt in explicitly with `runs-on: [self-hosted, linux, bobsled]`. Without this, an unrelated self-hosted runner attached to the same repo could pick up jobs intended for the hardened wrapper.
- **`podman`** — capability marker. Signals that PiP works in the wrapper. Workflows that need `docker`/`podman` should require it.

Pool-specific labels (e.g. `bar-secrets`, `gpu`, `large`) are operator-defined per pool in `inventory.yaml` and live alongside the conventional two.

**Runner name** convention: `bobsled-<host>-<slot>` (e.g. `bobsled-h1-7`). This encodes provenance so `bobsled gc` can identify orphans by name prefix, and so each registration on the GitHub side is greppable back to its source host and slot.

## Data flow

### Runner startup (the hot loop, runs after every job)

```
user-systemd starts bobsled@7.service
  └─► ExecStartPre: ~/bin/bobsled-mint --instance 7 \
                       --output /run/user/<uid>/bobsled/7/jit.json
        ├─ load ~/config.yaml
        ├─ load ~/state.yaml; look up instance 7 (e.g., repo=acme/foo)
        ├─ mkdir -p ~/.cache/bobsled/slots/7/acme--foo/
        ├─ atomically repoint ~/.cache/bobsled/slots/7/current → acme--foo/
        ├─ JWT-sign with app key → installation token
        ├─ POST .../actions/runners/generate-jitconfig
        └─ write JSON to /run/user/<uid>/bobsled/7/jit.json
  └─► ExecStart: podman run ... wrapper image
        └─ entrypoint reads /jit/jit.json, runs ./run.sh --jitconfig "..."
           └─ runner registers (JIT), picks up one job, exits
  └─► ExecStopPost: rm -rf /run/user/<uid>/bobsled/7
  └─► Restart=always → loop
```

### `bobsled apply -f inventory.yaml`

```
1. Parse inventory.
2. For each host (in parallel, SSH as bobsled@host):
     a. Read current ~/state.yaml.
     b. Compute desired state from pools[].spread allocation.
     c. Diff:
        - removed instances → systemctl --user disable --now bobsled@N
        - changed instances (repo/labels) → drain old, write new state, restart
        - new instances → write state, systemctl --user enable --now bobsled@N
     d. Atomically rewrite ~/state.yaml.
3. Print per-host diff + fleet status.
```

A per-host flock on `state.yaml` prevents two concurrent `apply` invocations from racing.

### Drain

`systemctl --user disable bobsled@N` stops the auto-restart but does not interrupt a running job. The unit exits naturally when its current job finishes and does not come back. `bobsled drain` polls until all targeted units are `inactive`.

## Cache

Caches are keyed by **(slot, repo)**. Layout on disk:

```
~/.cache/bobsled/slots/<N>/
  ├── <owner>--<repo>/       # one dir per repo this slot has ever served
  │     └── podman-storage/  # inner podman graph root for (slot, repo)
  └── current → <owner>--<repo>/   # symlink, updated atomically by mint
```

The systemd unit mounts `~/.cache/bobsled/slots/%i/current` into the wrapper at `/cache`. Podman resolves the symlink at container start, so the running container sees the correct repo-specific directory.

Lifecycle:

- Owned by `bobsled`, mode 0700.
- Inner podman's graph root is `/cache/podman-storage`, so `docker pull` / `docker build` inside the wrapper benefits from a warm layer cache across consecutive jobs in the same (slot, repo).
- Workflows can also use `/cache` for their own caches (npm, pip, cargo, buildx, etc.).
- On each runner startup, `bobsled-mint` ensures the per-(slot, repo) dir exists and atomically repoints `current` at it. Slot reassignment (e.g., slot 7 moved from `acme/foo` to `acme/bar` via `apply`) automatically switches to a fresh `acme--bar/` cache on the next restart, without touching `acme/foo`'s cache.
- `bobsled cache gc` deletes non-current `<repo>/` dirs to reclaim space.
- `bobsled cache reset` wipes either a single (slot, repo), a whole slot, or the entire host.

Why per-(slot, repo) rather than per-slot or per-repo:

- **Per-slot only** (alternative we considered): simpler but a slot reassignment silently inherits the old repo's stale layers and tooling caches. Possible information leak between repos.
- **Per-repo only** (alternative): multiple slots serving the same repo would write the inner podman storage concurrently. Storage drivers' locking is fragile under this load; one bad job can poison the cache for every concurrent runner.
- **Per-(slot, repo)** (this design): each `bobsled@N` is serialized by systemd, so within a (slot, repo) there's never a concurrent writer. Slot reassignment is clean. Cost: cache benefit is per (slot, repo), not pooled across slots. Acceptable at this scale; a pull-through registry mirror can be added later for cross-slot image cache.

## Error handling

| Failure | Behavior |
|---|---|
| `bobsled-mint` fails (network, API down, key expired, rate-limit) | `ExecStartPre` non-zero → systemd restarts per `RestartSec=2s`. After `StartLimitBurst=10` in `StartLimitIntervalSec=120`, unit goes to `failed`; `bobsled ls` surfaces it. |
| Container starts but actions/runner can't reach GitHub | Container exits → systemd restarts → same start-limit catches sustained outages. |
| Job hangs | Out of scope for the orchestrator; GitHub enforces a 6-hour max. Optional per-unit `RuntimeMaxSec=` for a hard ceiling. |
| Host reboot | Units `enabled` → start on boot → mint → run. State.yaml survives in `/var/lib/`. |
| Two `apply` invocations concurrently | Per-host flock on `state.yaml`; second invocation waits or fails loudly. |
| Orphan runners on GitHub side (host disappeared without draining) | `bobsled gc` reconciles via API, deletes strays whose names match the inventory's prefix but aren't currently expected. |
| Unused JIT config (mint succeeded, container crashed before `run.sh`) | JIT configs are single-use; an unused one expires harmlessly. No cleanup required. |
| Image upgrade mid-job | `host upgrade` updates the pinned digest but does not restart running units. Next natural restart (after current job) picks up the new digest. |

## Security model

### Isolation layers (workflow code → host root)

1. Outer rootless podman container (the wrapper): launched by user-level systemd as the `bobsled` user, user-namespaced, read-only root FS, dropped caps, default seccomp, `no-new-privileges`.
2. Inner podman: fuse-overlayfs + its own user-namespace mapping inside the wrapper.
3. Workflow processes inside inner podman containers.

Workflow code that escapes to the outer runner is still an unprivileged user inside a container; an escape from *that* lands on a non-root host user (`bobsled`), whose only powers are over its own home and runtime dirs. Two boundaries between workflow code and host root, and the orchestrator itself never runs anything as root at runtime.

### GitHub App key

- Mode 0600, owned by the `bobsled` system user.
- Only `bobsled-mint` (running as that user) reads it.
- App scoped to `Administration: write` + `Actions: read` on selected repos only.
- Rotated via `bobsled host rotate-key`.

### SSH

The CLI shells out to `ssh`/`scp`. Operator's existing SSH config and agent apply. Two SSH targets per host:

- `bootstrap_ssh` (admin user): only used by `host bootstrap` for the one-time privileged setup.
- `ssh` (`bobsled@host`): used by every other CLI operation. Authorized keys are seeded during bootstrap. This account has no sudo and no shell access to anything outside `/var/lib/bobsled/` and `~/.cache/bobsled/`.

### Network

- Outer container: default rootless podman networking (slirp4netns). Egress to GitHub and package registries.
- `bobsled-mint`: binds nothing. It is a CLI invocation, not a service.
- No inbound ports opened on hosts as part of this system.

## Testing

- **Unit tests:** mint binary (JWT signing logic, JIT API call with mocked HTTP), state.yaml parsing/writing, inventory diffing / allocation.
- **Integration test on a single VM:** vagrant or lima box plus a throwaway test repo. CI runs `bobsled host bootstrap`, `bobsled apply` with count=2, triggers a workflow on the test repo, asserts both runners pick up jobs and are replaced. Same VM covers `drain`, `gc`, `host upgrade`, `cache reset`.
- **Manual multi-host check before tagging a release:** run against the actual fleet (1–3 hosts) with a test repo.
- GitHub API is **not** mocked in integration tests; tests hit the real API against the test repo. Registration is a side-effecty boundary where mocks lie.

## Open questions / deferred

- Pull-through registry mirror for cross-slot image cache: deferred until the per-slot cache proves insufficient.
- Metrics / observability beyond `bobsled ls`: deferred.
- Untrusted-PR threat model: not addressed; would require additional hardening (firejail-style policies, network egress controls).
- Windows / macOS runners: out of scope.

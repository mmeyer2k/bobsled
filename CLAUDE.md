# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**bobsled** orchestrates a small fleet (1–3 hosts, 5–20 runners) of self-hosted, ephemeral, podman-in-podman GitHub Actions runners. The operator runs a Go CLI on their workstation; it SSHes to each host as `bobsled@host` and drives `systemctl --user`. Each unit launches one rootless podman container per job, which mints a fresh JIT runner config from a GitHub App, runs one job, then exits — systemd's `Restart=always` brings up a fresh container.

Public repo: https://github.com/mmeyer2k/bobsled

## Authoritative documents

- **Spec:** `docs/superpowers/specs/2026-05-13-bobsled-design.md` — design decisions, threat model, file layout, error handling. Re-read the relevant section before making non-trivial changes.
- **Plan:** `docs/superpowers/plans/2026-05-13-bobsled.md` — the original 32-task TDD implementation plan. Useful for understanding why specific code is shaped the way it is.

When the spec and code disagree, the spec wins unless there's a documented `fix(...)` commit explaining the divergence.

## Build, test, lint

Go is installed at `/home/mike/.local/go/bin/go`, **not on the default PATH**. Either prefix every invocation or export:

```bash
export PATH=/home/mike/.local/go/bin:$PATH
```

Then:

```bash
make build       # builds bin/bobsled and bin/bobsled-mint (runs `make assets` first)
make test        # go test ./...
make test-race   # go test -race ./...
make lint        # go vet ./...
make clean       # rm -rf ./bin
```

Single-package test:
```bash
go test ./internal/ghapp/... -v
go test -run TestSignAppJWT ./internal/ghapp/... -v
```

Build the wrapper image (requires podman):
```bash
./scripts/build-image.sh   # prints sha256:<digest> on success
```

End-to-end smoke (requires a real GitHub App + test repo + env vars — see script):
```bash
./scripts/smoke.sh
```

## Architecture

Two Go binaries:

- **`cmd/bobsled`** — operator CLI. Cobra-based; subcommands in `internal/cli/*.go` (one file per subcommand or subcommand group). Shells out to `ssh`/`scp` via `internal/ssh`. Stateless; the only operator-side state is `inventory.yaml` and the GitHub App private key.

- **`cmd/bobsled-mint`** — on-host one-shot binary. Invoked from `ExecStartPre=` in the systemd unit. Reads `~/config.yaml` + `~/state.yaml`, ensures the per-(slot, repo) cache symlink, mints a JIT runner config from GitHub, writes it to `--output`. Exits non-zero on any failure so systemd retries per its restart policy. Orchestration lives in `internal/runner/mint.go`.

Supporting packages under `internal/`:

| Package | Role |
|---|---|
| `config` | Parse on-host `config.yaml` (app_id, app_key_path, host_label, github_api). |
| `state` | Per-host `state.yaml`: which slot serves which repo, plus per-repo label sets. Atomic write via same-dir `os.CreateTemp` + `os.Rename`. |
| `inventory` | Operator-side `inventory.yaml`: GitHub App creds, hosts, pools. Includes deterministic greedy `Allocate(inv)` and a `DiffStates(cur, want)` helper used by `apply`. |
| `ghapp` | GitHub App auth: PKCS1+PKCS8 JWT signing, installation token fetch, JIT-config mint, list/delete repo runners. All HTTP through a `Client` struct with injectable `HTTP` + `Now` for tests. |
| `cache` | `EnsureCurrent(slotDir, repo)` — creates `slotDir/<repo-slug>/` and atomically repoints `slotDir/current` at it via a same-dir rename of a temp symlink. `RepoSlug("owner/repo")` returns `"owner--repo"`. |
| `runner` | `Mint(ctx, Options)` — wraps the four packages above into the on-host hot loop. |
| `ssh` | Thin wrappers around `ssh` / `scp`. `Run(cmd)`, `PutFile(local, remote)`, `PutBytes(data, remote)`. |
| `cli` | Cobra subcommands. |

### Host-side runtime layout (per the spec)

```
~bobsled/                                    # /var/lib/bobsled
  config.yaml, state.yaml, app-key.pem, image-digest.env
  .local/bin/bobsled-mint
  .config/systemd/user/bobsled@.service
  .cache/bobsled/slots/<N>/<repo-slug>/      # the (slot, repo) cache
                          podman-storage/    # inner podman graph root
                       current → <repo-slug> # mint-managed symlink
```

The systemd template unit (`systemd/bobsled@.service`) is the only thing that talks to podman directly. It mounts the slot's `current` symlink at `/cache` inside the container; podman resolves the symlink at start, so the running container sees the correct repo-specific dir.

### Hot loop (one job per restart)

1. `systemctl --user start bobsled@N` triggers `ExecStartPre=bobsled-mint --instance N --output …`.
2. mint loads config + state, ensures `~/.cache/bobsled/slots/N/<repo>/` exists, repoints `current`, calls GitHub's `generate-jitconfig`, writes JSON to `%t/bobsled/N/jit.json`.
3. `ExecStart=podman run … bobsled:${BOBSLED_IMAGE_DIGEST}` launches the wrapper container; its entrypoint reads `/jit/jit.json`, exec's `./run.sh --jitconfig "$cfg"`, runner takes one job, exits.
4. `Restart=always` → loop.

## Conventions worth knowing

- **Atomic writes everywhere** that state can be read concurrently: write to `<path>.tmp` then `os.Rename`. This pattern is in `state.Write`, `cache.EnsureCurrent`, `mint.Mint`, and the SSH-side `apply`/`scale` (`flock -x state.yaml -c 'mv .state.yaml.tmp state.yaml'`). Before flock'ing on state.yaml over SSH, `touch state.yaml` first so flock has a file (fresh-host case).
- **TDD is the norm.** Every non-trivial package has a `*_test.go` with the test written before the implementation. The plan documents this discipline; preserve it.
- **`net/http` directly** for GitHub — no `go-github` SDK. Keep the surface small.
- **No mocks at the GitHub boundary** in tests; use `httptest.NewServer` to spin up a real handler. The test in `internal/runner/mint_test.go` is the canonical example.
- **Runner naming convention:** `bobsled-<host>-<slot>` (e.g. `bobsled-h1-7`). Encoded in `mint.Mint`. `gc` uses the `bobsled-` prefix to identify orphans owned by this orchestrator.
- **GitHub labels convention:** **JIT runner registration does NOT auto-add `self-hosted`/`linux`/`x64`** (verified by smoke test — those auto-labels only attach via the classic `./config.sh` registration flow). Anything you want on the runner must be in `state.yaml`'s labels list. Recommend `[self-hosted, linux, x64, bobsled, podman]` plus any pool-specific extras. The old "don't pass auto labels" advice was wrong for JIT.
- **`go:embed` for assets.** `systemd/bobsled@.service` and `assets/bootstrap.sh` are embedded by `assets/assets.go`. The Makefile's `assets` target copies the canonical unit file into `assets/` before `go build` so the embed compiles. If you edit the unit, run `make assets` (or `make build`) before committing.

## Subcommand surface

```
bobsled host bootstrap <host>           # one-shot privileged setup (uses bootstrap_ssh)
bobsled host install <host> --image-digest sha256:...
bobsled host upgrade <host> [--mint-binary ...] [--image-digest ...]
bobsled host rotate-key <host> --key <local-pem>
bobsled apply                            # declarative reconcile across all hosts
bobsled scale --host h1 --repo o/r --count N [--labels ...]
bobsled drain --host h1 [--slot N]
bobsled ls
bobsled gc [--dry-run]                  # delete orphan GitHub-side runners
bobsled cache reset --host h1 [--slot N] [--repo o/r]
bobsled cache gc --host h1
bobsled image build                     # wraps scripts/build-image.sh
bobsled image push <digest> --registry <repo>
```

All commands accept `-i/--inventory <path>` (default `inventory.yaml`).

## Known follow-ups (non-blocking)

Captured in the final review of the initial implementation:

- `ssh.Client.Run` uses `Output()` not `CombinedOutput()` — stderr is dropped on success. Worth fixing for `systemctl --user` debugging.
- `ls` reads `state.yaml` without `flock` (inconsistent with `apply` / `scale`).
- `gc` reconciles against the desired allocation, not each host's `state.yaml` — could delete in-flight runners during apply drift.
- `scale` doesn't `MarkFlagRequired("count")` in cobra; the manual `count < 0` check covers it but the UX is inconsistent with other required flags.
- **409 conflict on restart**: if the wrapper container fails before consuming its JIT (e.g. mid-job crash), GitHub still has the runner registered, and the next mint hits `409 A runner with the name *** already exists`, restart-looping until `StartLimitBurst`. Mint should detect 409, look up the conflicting runner by name, delete it, and retry. `ghapp.ListRepoRunners` + `DeleteRepoRunner` are already wired — just need the error-path glue in `internal/runner/mint.go`.

## Threat model boundaries (don't relax without revisiting the spec)

- Wrapper container runs `--userns=keep-id:uid=1000,gid=1000` (so JIT files written by the bobsled user are readable inside as `podman`) with `--tmpfs=/tmp` and `--device=/dev/fuse` (for the inner fuse-overlayfs). The container is ephemeral (`--rm`, one job, ~minutes).
- **What we DON'T enforce, and why:** `--read-only` breaks actions/runner ($HOME writes). `--cap-drop=ALL` + `--security-opt=no-new-privileges` break the inner rootless podman (setuid `newuidmap` can't elevate). The threat model trades these for PiP support — the container is still user-namespaced and short-lived, and the spec's non-goal of "untrusted PR workflows" is what makes that trade acceptable.
- The GitHub App key lives at `~bobsled/app-key.pem` mode 0600, readable only by the `bobsled` system user. Only `bobsled-mint` reads it.
- The orchestrator never runs anything as root at runtime. `host bootstrap` is the only command that needs admin SSH; everything else connects as `bobsled@host`.

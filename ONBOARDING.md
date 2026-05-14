# Onboarding a bobsled host

This is the **first-time** walkthrough that takes you from "fresh Linux box" → "self-hosted Actions runner picking up jobs." It's split into three phases: **install**, **wire up GitHub**, **light it up**.

It documents the path we walked end-to-end the first time we tested. Every step is marked **AUTO** (script can do it), **AUTO once setup**, or **MANUAL** (web UI, decisions, secrets).

Throughout, replace placeholders:
- `<repo>` → `mmeyer2k/bobsled-smoke` (your test repo)
- `<app-id>` → the GitHub App's numeric ID
- `<app-key>` → local path to the App's downloaded `.pem`
- `<host>` → `local` (or whatever you call this machine in `inventory.yaml`)

> **Local vs remote.** The full design is "CLI on operator's laptop SSHes to remote hosts as `bobsled@host`." For local kicking-the-tires we skip SSH entirely and just `sudo -iu bobsled` to do the same work. The on-host artifacts are identical either way.

---

## Phase 1 — Install on the host

### 1.1 — Prerequisites *(AUTO)*

Linux with systemd ≥ 240, passwordless `sudo` for the admin user. The bootstrap script handles the rest:

- **`podman` ≥ 4.x**, `fuse-overlayfs`, `slirp4netns`, `uidmap`
- **`bobsled` system user** with home `/var/lib/bobsled`, real shell, lingering enabled
- Subuid/subgid ranges allocated for `bobsled`
- Bobsled-owned dirs: `~/.ssh`, `~/.local/bin`, `~/.cache/bobsled`, `~/.config/systemd/user`

If your host doesn't have podman 4+, the script installs it. Test boxes that already have `gh` and podman skip almost everything.

### 1.2 — Run bootstrap *(AUTO)*

From a checkout of `bobsled`:

```bash
bash assets/bootstrap.sh
```

The script is idempotent — re-running on an already-bootstrapped host is safe.

**Verify:**
```bash
id bobsled                                 # user exists, UID assigned
loginctl show-user bobsled -p Linger       # Linger=yes
sudo ls /var/lib/bobsled/.local/bin /var/lib/bobsled/.config/systemd/user
```

### 1.3 — Build the wrapper image *(AUTO)*

The image is `quay.io/podman/stable` + the pinned `actions/runner` tarball + `libicu` + a thin entrypoint script.

```bash
# As your user (any user with podman):
./scripts/build-image.sh
# Output ends with: sha256:<digest>
```

**Two gotchas you only learn the hard way:**

- The base image already has a user at UID 1000 (`podman`). Don't try to `useradd` over it.
- The .NET runner aborts at startup without `libicu`. The Containerfile already installs it.

You can build under your normal user, then transfer to the `bobsled` user's storage (next step). Or build directly as `bobsled` — but the source tree at `~/Code/bobsled/` is typically `750` so `bobsled` can't traverse `/home/<you>`.

The cleanest local path: copy `image/` into `/tmp/bobsled-image` (readable by bobsled), then `sudo -iu bobsled` and build there:

```bash
sudo cp -r ./image /tmp/bobsled-image
sudo chown -R bobsled:bobsled /tmp/bobsled-image
sudo -iu bobsled bash -lc 'cd /tmp/bobsled-image && podman build \
    --build-arg RUNNER_VERSION=$(cat runner-version) \
    --iidfile /tmp/bobsled-iid -t bobsled:local .'
```

Tag the image by its content digest so the systemd unit can resolve it:

```bash
IID=$(sudo cat /tmp/bobsled-iid | sed 's/sha256://')
sudo -iu bobsled podman tag bobsled:local "bobsled:${IID}"
echo "IID=$IID"
```

### 1.4 — Stage the mint binary + systemd unit in `~bobsled/` *(AUTO)*

Build the binary:

```bash
make build   # produces ./bin/bobsled and ./bin/bobsled-mint
```

Install into bobsled's home:

```bash
sudo install -o bobsled -g bobsled -m 0755 ./bin/bobsled-mint \
    /var/lib/bobsled/.local/bin/bobsled-mint
sudo install -o bobsled -g bobsled -m 0644 ./systemd/bobsled@.service \
    /var/lib/bobsled/.config/systemd/user/bobsled@.service
sudo -iu bobsled systemctl --user daemon-reload
```

**Verify:**
```bash
sudo -iu bobsled systemctl --user list-unit-files 'bobsled@*'
# Expect: bobsled@.service  disabled  enabled
```

---

## Phase 2 — Wire up GitHub

### 2.1 — Create a test repo with a smoke workflow *(AUTO once)*

```bash
gh repo create mmeyer2k/bobsled-smoke --private --description "bobsled smoke test"
cd $(mktemp -d) && gh repo clone mmeyer2k/bobsled-smoke && cd bobsled-smoke
mkdir -p .github/workflows
cat > .github/workflows/ci.yml <<'EOF'
name: smoke
on: workflow_dispatch
jobs:
  hello:
    runs-on: [self-hosted, linux, bobsled]
    steps:
      - run: |
          uname -a
          id
          podman --version
          podman run --rm docker.io/library/alpine:3.21 sh -c 'echo "inner container ran OK"'
EOF
git add . && git commit -m "ci: smoke workflow"
git push
```

**Gotcha:** if your local `user.email` is your real address and GitHub's "Block command line pushes that expose my email" is on, the push is rejected. Use the noreply form:

```bash
USER_ID=$(gh api user --jq .id)
git config user.email "${USER_ID}+$(gh api user --jq .login)@users.noreply.github.com"
git commit --amend --reset-author --no-edit
git push
```

### 2.2 — Create a GitHub App *(MANUAL)*

This is the only step the CLI can't help with — App creation is web-UI only.

1. Go to **https://github.com/settings/apps/new**
2. Fill in:
   - **Name:** `bobsled-runners-<your-handle>` (must be globally unique)
   - **Homepage URL:** anything (your repo URL is fine)
   - **Webhook:** Uncheck "Active"
   - **Repository permissions → Administration:** Read & write
   - **Repository permissions → Actions:** Read-only
   - **Where can this be installed:** "Only on this account"
3. Click **Create GitHub App**
4. On the App's settings page, note the **App ID** (top of the page)
5. Scroll to **Private keys** → **Generate a private key** → a `.pem` downloads
6. Left sidebar → **Install App** → install on the test repo only

**Verify (without leaving the terminal):**
```bash
ls ~/Downloads/*bobsled*.pem   # confirm key downloaded
```

You won't be able to look up the App ID via `gh api /apps/<slug>` because personal-account Apps aren't publicly visible. Read it from the App settings page.

### 2.3 — Place the App key + write config + state *(AUTO)*

```bash
APP_ID=<app-id>
APP_KEY_SRC=<path-to-downloaded.pem>
TEST_REPO=mmeyer2k/bobsled-smoke

sudo install -o bobsled -g bobsled -m 0600 "$APP_KEY_SRC" /var/lib/bobsled/app-key.pem

sudo -u bobsled tee /var/lib/bobsled/config.yaml >/dev/null <<EOF
app_id: ${APP_ID}
app_key_path: /var/lib/bobsled/app-key.pem
host_label: local
EOF

sudo -u bobsled tee /var/lib/bobsled/state.yaml >/dev/null <<EOF
repos:
  ${TEST_REPO}:
    labels: [self-hosted, linux, x64, bobsled, podman]
instances:
  1: {repo: ${TEST_REPO}}
EOF

# Wire the image digest the systemd unit will reference
sudo -u bobsled tee /var/lib/bobsled/image-digest.env >/dev/null <<EOF
BOBSLED_IMAGE_DIGEST=${IID}
EOF
```

> **Labels gotcha.** With JIT runner registration (what bobsled uses), GitHub does **NOT** auto-add `self-hosted`/`linux`/`x64` like it does for classic `./config.sh` registration. Anything you want on the runner must be in this `labels:` list. Without these, `runs-on: [self-hosted, linux, bobsled]` won't match.

### 2.4 — Sanity-mint *(AUTO)*

Verify the App + key + state are wired correctly before lighting up systemd:

```bash
sudo -iu bobsled mkdir -p /run/user/$(id -u bobsled)/bobsled/1
sudo -iu bobsled /var/lib/bobsled/.local/bin/bobsled-mint --instance 1 \
    --output /run/user/$(id -u bobsled)/bobsled/1/jit.json
```

**Expect:** exit 0, no output, `/run/user/.../jit.json` contains `{"runner":{...},"encoded_jit_config":"..."}`.

If you get **409 Conflict — A runner with the name *** already exists**, you have a stale registered runner on GitHub. Delete it and retry:

```bash
gh api /repos/${TEST_REPO}/actions/runners --jq '.runners[].id' \
    | xargs -I{} gh api -X DELETE /repos/${TEST_REPO}/actions/runners/{}
```

The 409 path is a [known follow-up](CLAUDE.md#known-follow-ups-non-blocking) — mint should handle this itself but doesn't yet.

---

## Phase 3 — Light it up

### 3.1 — Enable the unit *(AUTO)*

```bash
sudo -iu bobsled systemctl --user enable --now bobsled@1
sudo -iu bobsled systemctl --user status bobsled@1 --no-pager | head -10
```

Within a couple of seconds the runner appears online:

```bash
gh api /repos/${TEST_REPO}/actions/runners --jq '.runners[]'
# {"id": ..., "name": "bobsled-local-1", "status": "online", "labels": [...]}
```

### 3.2 — Trigger a workflow *(AUTO)*

```bash
gh workflow run -R ${TEST_REPO} ci.yml
sleep 5
gh run list -R ${TEST_REPO} --limit 1
```

Watch it:

```bash
RUN_ID=$(gh run list -R ${TEST_REPO} --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch -R ${TEST_REPO} ${RUN_ID}
```

**Expect:** the job picks up within seconds, runs (`uname` / `podman --version` / inner Alpine container pull + run), and finishes with `success`. The wrapper container exits, systemd restarts the unit, a NEW runner registers (different ID, same name), ready for the next job.

### 3.3 — Watching the hot loop *(AUTO)*

```bash
# In one terminal — watch unit invocations come and go
sudo journalctl _UID=$(id -u bobsled) -f \
    | grep -E 'bobsled@1|bobsled-mint|Started|Failed|exit'

# In another — watch GH-side runner IDs roll
watch -n 2 'gh api /repos/'${TEST_REPO}'/actions/runners --jq ".runners[] | {id, name, status, busy}"'
```

After every job: ID increments, status flips offline → online again, busy returns to false.

---

## Automation potential

| Step | Currently | Future state |
|---|---|---|
| 1.1 Prerequisites detection | AUTO (in bootstrap.sh) | Same |
| 1.2 Bootstrap | AUTO (`bash assets/bootstrap.sh`) | Same |
| 1.3 Image build + transfer | AUTO (`scripts/build-image.sh` + tag) | Wrap behind `bobsled image build` (already exists; needs local-only mode) |
| 1.4 Stage binary + unit | AUTO (manual scp-equivalent now) | `bobsled host install` already does this over SSH; add a `--local` mode for sudo-su |
| 2.1 Test repo + workflow | AUTO once | One-shot script; idempotent |
| **2.2 GitHub App creation** | **MANUAL** | Cannot automate — GitHub's web UI only. Best we can do is a checklist. |
| 2.3 Place key + config + state | AUTO | Could be a `bobsled local-install` subcommand that takes `--app-id`, `--app-key`, `--repo` |
| 2.4 Sanity-mint | AUTO | Add a `bobsled doctor` subcommand that runs the mint check + reports failures with hints |
| 3.1 Enable unit | AUTO | Same |
| 3.2 Trigger workflow | AUTO | `gh` already covers this |

The natural next step is a `scripts/local-onboard.sh` that does steps 1.2 → 1.4 → 2.3 → 3.1 in one shot, given the App ID + key + repo. Step 2.2 will always be manual.

---

## Gotchas that bit us during the first run

These all became fixes/docs in the repo, but listing them here saves the next person the debugging:

| Symptom | Root cause | Fix |
|---|---|---|
| Container starts, exits in 6 ms, no logs | `--read-only` blocks actions/runner from writing helper files to `$HOME` | Drop `--read-only` from the unit |
| `Couldn't find a valid ICU package` | Wrapper image missing libicu (required by .NET runner) | Containerfile installs `libicu` + `krb5-libs` + `openssl-libs` + `zlib` |
| `Runner version v2.328.0 is deprecated and cannot receive messages` | Pinned runner version too old | Bump `image/runner-version`; current pin works for ~90 days, then GitHub starts refusing |
| Job stays queued forever despite runner being `online` | JIT-registered runner has only the labels you passed; `self-hosted`/`linux`/`x64` are NOT auto-added | Include them in `state.yaml`'s `labels:` list |
| Inner podman: `newuidmap: operation not permitted` | `--cap-drop=ALL` + `--security-opt=no-new-privileges` blocks setuid `newuidmap` from elevating | Drop both flags from the unit. Keep `--userns=keep-id:uid=1000,gid=1000` |
| `/jit/jit.json missing or unreadable` inside container | `--userns=keep-id` maps host UID to itself, but container's default user is `podman` (UID 1000), so files written by `bobsled` (UID 997) are foreign | `--userns=keep-id:uid=1000,gid=1000` (explicit mapping) |
| Container can't `mkfs.fuse` / fuse-overlayfs missing | `/dev/fuse` not exposed to the container | `--device=/dev/fuse` in the unit |
| 409 on every restart after a crash | Container exited before consuming the JIT, runner stays registered, next mint hits name collision | Manually delete via API (see 2.4). Real fix is in `mint.go` — pending. |
| `.cache`/`.config` parent dirs are root-owned | `install -d` only sets ownership on the leaf dir; intermediate parents get root | Bootstrap script now uses `sudo -u bobsled mkdir -p` instead |

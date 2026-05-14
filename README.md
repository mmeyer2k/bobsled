# bobsled

Orchestrator for self-hosted, ephemeral, podman-in-podman GitHub Actions runners.

## Quickstart

1. **Create a GitHub App** with:
   - Repository permissions: `Administration: write`, `Actions: read`
   - Install on the repos you want runners for
   - Download the private key

2. **Write `inventory.yaml`**:

   ```yaml
   github:
     app_id: 123456
     app_key: ~/keys/bobsled.pem

   hosts:
     h1:
       ssh: bobsled@runner-1.lan
       bootstrap_ssh: mike@runner-1.lan
       capacity: 8

   pools:
     - repo: acme/foo
       count: 6
       labels: [bobsled, podman]
       spread: [h1]
   ```

3. **Bootstrap each host** (one-time, admin SSH):

   ```bash
   ./bin/bobsled host bootstrap h1
   ```

4. **Build the wrapper image:**

   ```bash
   DIGEST=$(./bin/bobsled image build | tail -1)
   ```

5. **Install on each host:**

   ```bash
   ./bin/bobsled host install h1 --image-digest "${DIGEST}"
   ```

6. **Apply:**

   ```bash
   ./bin/bobsled apply
   ./bin/bobsled ls
   ```

## Day-2 ops

| Task | Command |
|---|---|
| Resize | edit `inventory.yaml`; `bobsled apply` |
| Drain a host | `bobsled drain --host h1` |
| Upgrade binary/image | `bobsled host upgrade h1 --mint-binary ./bin/bobsled-mint --image-digest sha256:...` |
| Rotate App key | `bobsled host rotate-key h1 --key ./new-key.pem` |
| Wipe a slot's cache | `bobsled cache reset --host h1 --slot 7` |
| Reclaim stale repo caches | `bobsled cache gc --host h1` |
| Clean GitHub-side orphans | `bobsled gc` |

See `docs/superpowers/specs/2026-05-13-bobsled-design.md` for the design.

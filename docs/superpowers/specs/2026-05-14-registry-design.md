# bobsled pull-through registry — Design

**Date:** 2026-05-14
**Status:** Draft for review

## Purpose

Add a per-host pull-through container registry to the bobsled stack so that workflow `docker pull` / `podman pull` calls inside wrapper containers hit a local cache instead of the public internet. Two motivations:

1. **Avoid usage limits.** Docker Hub anonymous-pull limits are the most likely to bite first, but ghcr.io and quay.io can also throttle.
2. **Speed up builds.** Each unique layer is fetched from upstream at most once per host, then served at LAN speed for every subsequent job.

The cache is intentionally per-host. With 1–3 hosts and `inventory.yaml`-driven topology, per-host autonomy is consistent with the rest of bobsled's design (no host-to-host traffic, every host is self-contained).

## Goals

- Transparent to workflow authors: `docker pull alpine` keeps working as written; the redirect to the local cache lives entirely in podman client config.
- Same operational shape as the rest of bobsled: rootless podman, user-level systemd, embedded assets, declarative install/upgrade through the CLI.
- Three upstreams covered: `docker.io`, `ghcr.io`, `quay.io`.
- Anonymous upstream pulls only (no Docker Hub credentials on hosts in v1).
- Cache survives host reboots and bobsled upgrades; bounded by automatic GC.
- Failure of the cache must degrade builds, not break them — clients fall through to the canonical upstream when the mirror is unreachable.

## Non-goals

- **Multi-host cache sharing.** Each host has its own cache. A single shared cache would couple hosts and add a SPOF; the duplicated layer storage is acceptable at this fleet size.
- **Authenticated upstream pulls.** No Docker Hub paid-plan auth, no private-registry credentials proxied. Workflows that need to pull from private repos do so directly (registries.conf only mirrors the three public upstreams).
- **Pushing.** This is read-only pull-through. Operators who want to push their own images use the existing `bobsled image push` flow against an external registry.
- **Cache warming / prefetch.** Cache fills on first pull (on-demand). No background warming.
- **Untrusted-workflow hardening.** Same threat model as the rest of bobsled — workflows are trusted; the cache is not a security boundary.

## Software choice: zot

[zot](https://github.com/project-zot/zot) is a CNCF-incubating OCI-native registry. We use it because:

- **Multi-upstream sync in a single process.** The official `registry:2` (Distribution) supports pull-through, but its `proxy.remoteurl` only handles Docker Hub — ghcr.io and quay.io are out. zot's `extensions.sync` supports an arbitrary list of upstreams in one config.
- **Small footprint.** ~20 MB image, single static binary, no database.
- **OCI-spec compliant.** Plays cleanly with podman's `registries.conf` mirror semantics.
- **GC built in.** No cron / external script needed for cache eviction.

The zot image is pinned by sha256 digest, mirroring the `BOBSLED_IMAGE_DIGEST` pattern used for the wrapper image.

## Architecture

```
host (bobsled user, systemctl --user)
│
├── bobsled-registry.service        ← new, singleton, long-lived
│   └── podman run --replace --name=bobsled-registry
│         -p 127.0.0.1:5000:5000
│         -v %h/.cache/bobsled/registry:/var/lib/registry
│         -v %h/registry-config.json:/etc/zot/config.json:ro
│         ghcr.io/project-zot/zot-linux-amd64@sha256:<pinned>
│
└── bobsled@{1..N}.service           ← existing, ephemeral, modified
    └── podman run --rm --replace --name=bobsled-N
          --add-host=host.containers.internal:host-gateway      ← new
          -v %t/bobsled/N:/jit:ro
          -v %h/.cache/bobsled/slots/N/current:/cache
          -v %h/registries.conf:/home/podman/.config/containers/registries.conf:ro  ← new
          bobsled:${BOBSLED_IMAGE_DIGEST}
```

Both units run as the same unprivileged `bobsled` user. The registry's port is bound to `127.0.0.1` on the host (not exposed beyond loopback). Wrapper containers reach it via `host.containers.internal:5000`. The explicit `--add-host=host.containers.internal:host-gateway` flag makes the name resolve deterministically to the host's gateway IP regardless of whether podman is using pasta or slirp4netns as its networking backend.

### Data flow

```
GH Actions workflow step
    │
    ▼
docker pull alpine               ← inside wrapper container
    │
    ▼
inner podman + ~/.config/containers/registries.conf  ← bind-mounted
    │  mirror lookup: docker.io → host.containers.internal:5000/docker.io
    ▼
host network gateway             ← via slirp4netns / pasta
    │
    ▼
zot @ 127.0.0.1:5000 on host
    │
    ├── HIT:  serve blob from ~/.cache/bobsled/registry
    └── MISS: fetch from registry-1.docker.io, store, serve
```

### Path-prefix routing inside zot

zot has one HTTP listener but three configured upstreams. To disambiguate which upstream a request belongs to, we use path prefixes that exist only inside zot's storage and the client's `registries.conf`:

| Client writes | registries.conf rewrites to | zot proxies to |
|---|---|---|
| `docker.io/library/alpine` | `host.containers.internal:5000/docker.io/library/alpine` | `registry-1.docker.io/library/alpine` |
| `ghcr.io/foo/bar` | `host.containers.internal:5000/ghcr.io/foo/bar` | `ghcr.io/foo/bar` |
| `quay.io/baz/qux` | `host.containers.internal:5000/quay.io/baz/qux` | `quay.io/baz/qux` |

The prefix is invisible to the workflow author. They keep writing `docker pull alpine` (or fully-qualified upstream names); the rewrite happens inside podman as it consults registries.conf.

## Host layout additions

```
/var/lib/bobsled/                                 # bobsled's $HOME
  ├── registry-config.json                        # zot config, written at install
  ├── registries.conf                             # podman client config, bind-mounted into wrappers
  ├── registry-image-digest.env                   # BOBSLED_REGISTRY_DIGEST=sha256:...
  ├── .config/systemd/user/bobsled-registry.service
  └── .cache/bobsled/
        ├── registry/                             # zot persistent storage (mode 0700)
        └── slots/                                # existing per-slot cache (unchanged)
```

`registry-config.json` and `registries.conf` are rendered from embedded templates and written by `bobsled host install`. Both are recreated (not patched) on `bobsled host upgrade` — they're considered derived state.

## Embedded assets

Mirroring the existing `assets/` package conventions (`go:embed`):

| New asset | Notes |
|---|---|
| `assets/registry.service` | Singleton systemd template (no `@` instance suffix) |
| `assets/registry-config.json.tmpl` | zot config — Go `text/template`; renders the three sync entries and GC settings |
| `assets/registries.conf.tmpl` | podman client config — Go `text/template`; renders one `[[registry]]` block per upstream |

`make assets` is updated to copy `systemd/registry.service` into `assets/` alongside the existing wrapper unit, same pattern as today.

## Inventory additions

`inventory.yaml` gains an optional top-level block:

```yaml
registry:
  image_digest: sha256:<pinned>      # default lives in code, override here for upgrades
  gc_interval: 1h                    # zot extension config
  gc_retention: 336h                 # 14 days
  upstreams:                         # optional override; default is the three below
    - name: docker.io
      url: https://registry-1.docker.io
    - name: ghcr.io
      url: https://ghcr.io
    - name: quay.io
      url: https://quay.io
```

The `upstreams` list is only present if you want to override the defaults (e.g. add a fourth registry). Defaults are baked into code so the common case is zero inventory churn.

## CLI surface

New subcommand group `bobsled registry`:

```
bobsled registry status <host>   # systemctl --user status bobsled-registry, plus
                                 # `curl 127.0.0.1:5000/v2/_catalog` content listing
bobsled registry restart <host>  # systemctl --user restart bobsled-registry
bobsled registry gc <host>       # trigger zot's GC endpoint
```

Existing subcommands gain registry-aware behavior:

- `bobsled host bootstrap <host>` — also creates `~/.cache/bobsled/registry` (mode 0700).
- `bobsled host install <host>` — also installs `bobsled-registry.service`, writes `registry-config.json` and `registries.conf`, enables and starts the unit. Idempotent.
- `bobsled host upgrade <host> [--registry-digest sha256:…]` — swaps the zot image digest in `registry-image-digest.env` and restarts the registry unit. Decoupled from wrapper-image upgrades.
- `bobsled ls` — adds a `REGISTRY` column showing the registry unit's status (active/inactive/failed).

## Failure modes

| Failure | Behavior |
|---|---|
| zot container down | Mirror lookup times out; podman falls through to the canonical upstream. Builds slow but don't fail. We rely on podman's built-in mirror fallback (no custom retry). |
| Cache disk full | zot's GC extension runs hourly and removes blobs not referenced by any tag for >14 days. Operator can force a sweep via `bobsled registry gc <host>`. |
| Corrupted blob in cache | Deleting the offending file from `~/.cache/bobsled/registry` is sufficient — next pull re-fetches it. `bobsled registry restart <host>` is the recovery hammer. |
| Wrapper can't reach `host.containers.internal` | Explicit `--add-host=host.containers.internal:host-gateway` on the wrapper unit pins the name to the host's gateway IP regardless of networking backend. If networking is broken at a deeper level, the mirror times out and falls through to the upstream — same as "zot down". |
| Upstream unreachable on cache miss | zot returns the upstream's error verbatim. podman fall-through doesn't help here (the upstream is the upstream). Cached blobs continue to serve. |

## Threat model boundary

The registry is **not** a security boundary. It runs as the same `bobsled` user as the wrappers, exposes its port only on host loopback (no external listener), and only proxies three well-known public registries. Workflow code that wanted to exfiltrate via the registry could already do so by hitting the upstreams directly; the cache adds no new exposure.

## Testing

- **Unit (`internal/registry`):**
  - Render `registry-config.json` from inventory + defaults; assert against a golden file.
  - Render `registries.conf`; assert against a golden file.
  - Inventory parsing: optional `registry:` block, optional `upstreams:` override, default fallback.

- **Integration:**
  - Spin up zot in a tempdir against an `httptest.NewServer` upstream; pull through it with `podman` (or `skopeo copy` for headless); assert blobs land in the cache dir and a second pull serves from cache without hitting the upstream. Mirrors the no-mocks-at-the-boundary discipline used in `internal/runner/mint_test.go`.

- **Smoke (`scripts/smoke.sh`):**
  - Extend to install the registry on the local host.
  - Run a workflow that pulls `alpine`.
  - Verify a second run pulls from cache (compare blob mtimes / `_catalog` contents before and after).

## Known follow-ups (not in scope here)

- Authenticated upstream pulls (Docker Hub paid plan, private GHCR repos).
- Cross-host cache sharing (one zot, multiple hosts hit it). Worth revisiting if the fleet grows past ~5 hosts.
- Pushing to the local cache for ad-hoc image distribution.
- Web UI / dashboards for cache hit-rate observability.

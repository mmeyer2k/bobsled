#!/usr/bin/env bash
# scripts/smoke.sh
# End-to-end smoke test against a single localhost host + a real GitHub repo.
#
# Prereqs (set before running):
#   - GitHub App created with Administration: write + Actions: read on a test repo
#   - BOBSLED_APP_ID    = App ID
#   - BOBSLED_APP_KEY   = local path to App private key
#   - BOBSLED_TEST_REPO = owner/repo of the test repo
#   - The test repo contains .github/workflows/ci.yml with `runs-on: [self-hosted, linux, bobsled]`
#   - Local SSH to bobsled@localhost works after `host bootstrap`

set -euo pipefail

cat > /tmp/bobsled-smoke-inventory.yaml <<EOF
github:
  app_id: ${BOBSLED_APP_ID:?set BOBSLED_APP_ID}
  app_key: ${BOBSLED_APP_KEY:?set BOBSLED_APP_KEY}
hosts:
  local:
    ssh: bobsled@localhost
    bootstrap_ssh: ${USER}@localhost
    capacity: 2
pools:
  - repo: ${BOBSLED_TEST_REPO:?set BOBSLED_TEST_REPO}
    count: 1
    labels: [bobsled, podman]
    spread: [local]
EOF

./bin/bobsled --inventory /tmp/bobsled-smoke-inventory.yaml host bootstrap local
DIGEST=$(./bin/bobsled --inventory /tmp/bobsled-smoke-inventory.yaml image build | tail -1)
./bin/bobsled --inventory /tmp/bobsled-smoke-inventory.yaml host install local \
    --image-digest "${DIGEST}" --app-key "${BOBSLED_APP_KEY}"

# Registry installed and active?
ssh bobsled@localhost 'systemctl --user is-active bobsled-registry.service' \
    | grep -qx active || { echo "smoke: registry not active"; exit 1; }

# Registry's loopback /v2/ endpoint reachable from the host?
ssh bobsled@localhost 'curl -fsS http://127.0.0.1:5000/v2/' \
    || { echo "smoke: registry /v2/ not reachable"; exit 1; }

./bin/bobsled --inventory /tmp/bobsled-smoke-inventory.yaml apply

gh workflow run -R "${BOBSLED_TEST_REPO}" ci.yml
sleep 60
./bin/bobsled --inventory /tmp/bobsled-smoke-inventory.yaml ls

# Did the workflow's pull land blobs under registry/docker.io/?
ssh bobsled@localhost 'find /var/lib/bobsled/.cache/bobsled/registry/docker.io -mindepth 1 -maxdepth 4 -type d 2>/dev/null | head -1' \
    | grep -q . || { echo "smoke: nothing cached under registry/docker.io/"; exit 1; }

echo "smoke: registry pull-through verified — blobs found under registry/docker.io/"

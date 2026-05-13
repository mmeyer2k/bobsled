#!/usr/bin/env bash
set -euo pipefail

if [[ ! -r /jit/jit.json ]]; then
    echo "entrypoint: /jit/jit.json missing or unreadable" >&2
    exit 1
fi
cfg=$(jq -r .encoded_jit_config /jit/jit.json)
if [[ -z "$cfg" || "$cfg" == "null" ]]; then
    echo "entrypoint: encoded_jit_config missing" >&2
    exit 1
fi
mkdir -p /cache/podman-storage /tmp/podman-run
cd /home/podman
exec ./run.sh --jitconfig "$cfg"

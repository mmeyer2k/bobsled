#!/usr/bin/env bash
# scripts/build-image.sh
# Builds the bobsled wrapper image, tags it with its content digest, prints
# the digest. Wrapped by `bobsled image build`.

set -euo pipefail
cd "$(dirname "$0")/../image"

runner_version=$(cat runner-version)
image_name=${IMAGE_NAME:-bobsled}

podman build \
    --build-arg "RUNNER_VERSION=${runner_version}" \
    --iidfile /tmp/bobsled-iid.$$ \
    -t "${image_name}:build" .

iid=$(cat /tmp/bobsled-iid.$$)
rm -f /tmp/bobsled-iid.$$
podman tag "${image_name}:build" "${image_name}:${iid#sha256:}"
echo "${iid}"

#!/usr/bin/env bash
# assets/bootstrap.sh — runs on the remote host via ssh + bash -s. Assumes
# `sudo` works without a password for the invoking user.
set -euo pipefail

USER_NAME=bobsled
HOME_DIR=/var/lib/bobsled

if command -v dnf >/dev/null 2>&1; then
    sudo dnf -y install podman fuse-overlayfs slirp4netns shadow-utils
elif command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    sudo DEBIAN_FRONTEND=noninteractive apt-get -y install podman fuse-overlayfs slirp4netns uidmap
else
    echo "Unsupported package manager — install podman, fuse-overlayfs, slirp4netns manually" >&2
    exit 1
fi

if ! id "$USER_NAME" >/dev/null 2>&1; then
    sudo useradd --system --create-home --home-dir "$HOME_DIR" --shell /bin/bash "$USER_NAME"
fi
sudo chmod 0750 "$HOME_DIR"

grep -q "^${USER_NAME}:" /etc/subuid || echo "${USER_NAME}:200000:65536" | sudo tee -a /etc/subuid >/dev/null
grep -q "^${USER_NAME}:" /etc/subgid || echo "${USER_NAME}:200000:65536" | sudo tee -a /etc/subgid >/dev/null

sudo loginctl enable-linger "$USER_NAME"

# Create all bobsled-owned dirs as the bobsled user itself so intermediate
# parents (~/.cache, ~/.config) are bobsled-owned, not root-owned.
sudo -u "$USER_NAME" mkdir -p \
    "$HOME_DIR/.ssh" \
    "$HOME_DIR/.local/bin" \
    "$HOME_DIR/.cache/bobsled" \
    "$HOME_DIR/.cache/bobsled/registry" \
    "$HOME_DIR/.config/systemd/user"
sudo -u "$USER_NAME" chmod 0700 \
    "$HOME_DIR/.ssh" \
    "$HOME_DIR/.local" \
    "$HOME_DIR/.local/bin" \
    "$HOME_DIR/.cache/bobsled" \
    "$HOME_DIR/.cache/bobsled/registry"
sudo -u "$USER_NAME" touch "$HOME_DIR/.ssh/authorized_keys"
sudo -u "$USER_NAME" chmod 0600 "$HOME_DIR/.ssh/authorized_keys"

echo "bootstrap: $USER_NAME ready"

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

sudo install -d -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HOME_DIR/.ssh"
sudo install -d -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HOME_DIR/bin"
sudo install -d -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HOME_DIR/.cache/bobsled"
sudo install -d -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HOME_DIR/.config/systemd/user"
sudo touch "$HOME_DIR/.ssh/authorized_keys"
sudo chown "$USER_NAME:$USER_NAME" "$HOME_DIR/.ssh/authorized_keys"
sudo chmod 0600 "$HOME_DIR/.ssh/authorized_keys"

echo "bootstrap: $USER_NAME ready"

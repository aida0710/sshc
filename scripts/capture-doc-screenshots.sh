#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
visual_dir=$(mktemp -d "${TMPDIR:-/tmp}/sshc-doc-screenshots.XXXXXX")
trap 'rm -rf -- "$visual_dir"' EXIT HUP INT TERM

cd "$repo_dir"
make build

SSHC_VISUAL_DIR="$visual_dir" npm run e2e --prefix web -- \
  e2e/shell.spec.ts \
  e2e/connections.spec.ts \
  e2e/secrets.spec.ts \
  e2e/sftp-transfer.spec.ts \
  e2e/sync.spec.ts \
  e2e/workspace-drag.spec.ts \
  e2e/narrow.spec.ts \
  e2e/terminal-features.spec.ts \
  --workers=1 \
  --grep 'draws one separator above the desktop navigation version|draws one separator above the version in the mobile drawer|separates classification, filtered results, and connection detail without losing management controls|adds Local and SOCKS forwarding from the dedicated advanced view|gives one named secret to two hosts and writes neither name into the file|keeps a chunked SFTP upload visible while another section is open|shows push, preview, apply, persisted success, and a later failure as distinct results|docks connected terminals into a live workspace|broadcasts one command to two live local shells|keeps terminal actions compact and exposes terminal settings|renders the documented non-interactive CLI example'

install_image() {
  source_name=$1
  target_name=$2
  install -m 0644 "$visual_dir/$source_name" "$repo_dir/$target_name"
  printf '%s\n' "$target_name"
}

install_image home-desktop.png docs/images/home.png
install_image sshc-v0.16.2-mobile-home-dark.png pages/public/images/android-home.png
install_image cli-example-desktop.png pages/public/images/cli-desktop.png
install_image sshc-connections-management-desktop.png pages/public/images/connections-desktop.png
install_image sshc-connections-management-desktop.png pages/public/images/connections-management.png
install_image credentials-desktop.png pages/public/images/credentials-desktop.png
install_image port-forwarding-settings-desktop.png pages/public/images/port-forwarding.png
install_image sshc-v0.16.1-transfer-manager-desktop.png pages/public/images/sftp-desktop.png
install_image sync-desktop-en.png pages/public/images/sync-desktop-en.png
install_image sync-desktop-ja.png pages/public/images/sync-desktop-ja.png
install_image sync-desktop-en.png pages/public/images/sync-desktop.png
install_image sync-exclusions-desktop-en.png pages/public/images/sync-exclusions-desktop-en.png
install_image sync-exclusions-desktop-ja.png pages/public/images/sync-exclusions-desktop-ja.png
install_image sync-exclusions-mobile-ja.png pages/public/images/sync-exclusions-mobile-ja.png
install_image terminal-actions-desktop.png pages/public/images/terminal-actions.png
install_image terminal-desktop.png pages/public/images/terminal-desktop.png
install_image transfer-manager-en.png pages/public/images/transfer-manager-en.png
install_image transfer-manager-ja.png pages/public/images/transfer-manager-ja.png
install_image transfer-manager-en.png pages/public/images/transfer-manager.png
install_image sshc-v0.16.0-live-workspace-desktop.png pages/public/images/workspace-desktop.png

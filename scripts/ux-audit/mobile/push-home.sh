#!/bin/bash
# アプリを止め、files/.ssh を gen-home.py で作ったものに入れ替える（sshc の Vault と設定 files/.ssh/sshc は残す）。
# 使い方: push-home.sh <clean|rough> [--wipe]  （--wipe はアプリのデータを全部消して最初の起動に戻す）
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
adb=~/.local/share/android-sdk/platform-tools/adb
package=com.github.aida0710.sshc.dev
kind=${1:?usage: push-home.sh <clean|rough> [--wipe]}
"$adb" shell am force-stop "$package"
if [ "${2:-}" = "--wipe" ]; then "$adb" shell pm clear "$package" >/dev/null; fi
work=$(mktemp -d "$here/../home-XXXX")
python3 "$here/gen-home.py" "$kind" "$work" >/dev/null
# 既にある sshc/（Vault・設定）は残し、config と接続先のファイルと鍵だけ入れ替える
"$adb" shell "run-as $package sh -c 'mkdir -p files/.ssh && cd files/.ssh && rm -rf config connections id_ed25519_*'"
tar -C "$work" -cf - .ssh | "$adb" shell "run-as $package tar -xf - -C files"
"$adb" shell "run-as $package sh -c 'chmod 700 files/.ssh && ls files/.ssh'"
rm -rf "$work"

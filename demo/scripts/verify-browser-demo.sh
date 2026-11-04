#!/usr/bin/env bash
# ブラウザデモを組み立て、実際の VM で SSH 接続と SFTP の編集まで確かめる。PR の CI、Release、手元で同じ手順を流す。
# check-browser.mjs は画面の役割と日本語の名前でボタンを探すので、画面の形や文言を変えると落ちる。
#
# 必要なもの: Docker、cpio、Go、Node、Chromium（SSHC_DEMO_CHROMIUM で指定できる）。
# VERSION を指定すると、デモの sshc はそのバージョンを名乗る。Release が公開するタグを渡す。
set -euo pipefail

repository_directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd -- "$repository_directory"

npm ci --prefix demo
npm ci --prefix web
npm test --prefix demo
bash demo/scripts/build-images.sh
npm run build --prefix web -- --base=./ --outDir ../demo/images/ui
npm run build --prefix demo

server_log=$(mktemp)
node demo/scripts/serve.mjs > "$server_log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid"; rm -f -- "$server_log"' EXIT
node demo/scripts/check-browser.mjs

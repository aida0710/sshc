#!/usr/bin/env bash
# vpn-image-tag.sh は、エンジンが VPN のコンテナイメージに付けるタグ（internal/vpn の
# imageTag）を出力する。internal/vpn/container の各ファイルを名前のバイト順に並べ、
# 「container/<名前>」と中身を続けた並びの SHA-256 の、先頭12桁である。
#
# CI はこのタグでイメージを先に組み立てて読み込み、テストが同じイメージを使えるように
# する（テストはイメージがあれば組み立てない）。エンジンと食い違わないことは
# internal/vpn の TestTheCIImageTagScriptMatchesTheEngine が確かめる。
set -euo pipefail

directory="${1:-internal/vpn/container}"
cd "$directory"
for name in $(LC_ALL=C ls -A | LC_ALL=C sort); do
  printf 'container/%s' "$name"
  cat -- "$name"
done | sha256sum | cut -c1-12

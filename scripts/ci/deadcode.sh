#!/usr/bin/env bash
#
# 参照ゼロの関数を探す。
#
# OS 固有の参照を考慮し、Linux、macOS、Windows のすべてで到達不能な関数だけを
# 検出する。コンパイルできても本番から参照されないコードを確認するために使う。
set -euo pipefail

# **並べ方と比べ方を揃える。** sort はロケールの照合順で並べ、comm はバイトで
# 比べる。en_US.UTF-8 のような環境では両者が食い違い、comm は「並んでいない」と
# 言って止まる——実際、`internal/platform/windows/toolchain.go` が増えた日に
# 手元だけが赤くなった。CI のランナーは C ロケールなので、そこでは見えない。
export LC_ALL=C

cd "$(dirname "$0")/../.."

# tool 自体は host 向けに建てる。`go tool` に GOOS を渡すと tool ごと
# その OS 向けに建ててしまい、走らせられない。
tool="$(mktemp -d)/deadcode"
trap 'rm -rf "$(dirname "$tool")"' EXIT
go build -o "$tool" golang.org/x/tools/cmd/deadcode

work="$(mktemp -d)"
trap 'rm -rf "$(dirname "$tool")" "$work"' EXIT

# 各 OS について 2 つの一覧を取る: その OS の build に含まれる Go ファイルと、
# その OS で到達不能な関数。ある関数が死んでいるのは「それが建てられる
# すべての OS で到達不能」なときである。3 OS の報告の共通部分を取るだけでは、
# `_unix.go` や `_windows.go` にしか無い関数が「他の OS の一覧に無い」という
# 理由で共通部分から落ち、永久に検出されなかった。
for os in linux darwin windows; do
  # テスト専用の補助や interface 実装を除外するため -test を付ける。
  # node_modules には別のユーザーの Go が入っている。
  GOOS="$os" "$tool" -test ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | grep -v node_modules \
    | sed 's/:[0-9]*:[0-9]*: unreachable func: /\t/' \
    | sort > "$work/reported.$os"
  GOOS="$os" go list -f '{{$dir := .Dir}}{{range .GoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .TestGoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}' \
      ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | sed "s#^$(pwd)/##" \
    | sort > "$work/built.$os"
done

found="$work/found"
: > "$found"
sort -u "$work"/reported.* | while IFS=$'\t' read -r file symbol; do
  dead=1
  for os in linux darwin windows; do
    if grep -Fxq -- "$file" "$work/built.$os" && ! grep -Fxq -- "$file	$symbol" "$work/reported.$os"; then
      dead=0
      break
    fi
  done
  if [ "$dead" = 1 ]; then
    printf '%s\t%s\n' "$file" "$symbol" >> "$found"
  fi
done
sort -o "$found" "$found"

# 許容する到達不能シンボルには理由の記載を必須とする。
allowed="$(grep -v '^\s*#' scripts/ci/deadcode-allowed.tsv | grep -v '^\s*$' | cut -f1,2 | sort)"

unexpected="$(comm -23 "$found" <(printf '%s\n' "$allowed"))"
vanished="$(comm -13 "$found" <(printf '%s\n' "$allowed"))"

status=0
if [ -n "$unexpected" ]; then
  echo "参照ゼロの関数がある（建てられるすべての OS で到達不能）:" >&2
  printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
  echo "消すか、scripts/ci/deadcode-allowed.tsv に理由と一緒に書くこと。" >&2
  status=1
fi
if [ -n "$vanished" ]; then
  echo "許しの一覧に、もう死んでいないものが載っている:" >&2
  printf '%s\n' "$vanished" | sed 's/^/  /' >&2
  echo "scripts/ci/deadcode-allowed.tsv から消すこと。" >&2
  status=1
fi
[ "$status" = 0 ] && echo "deadcode: 参照ゼロの関数は無い"
exit "$status"

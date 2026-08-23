#!/usr/bin/env bash
#
# 参照ゼロの関数を探す。
#
# OS 固有の参照を考慮し、Linux、macOS、Windows のすべてで到達不能な関数だけを
# 検出する。コンパイルできても本番から参照されないコードを確認するために使う。
set -euo pipefail

cd "$(dirname "$0")/../.."

# tool 自体は host 向けに建てる。`go tool` に GOOS を渡すと tool ごと
# その OS 向けに建ててしまい、走らせられない。
tool="$(mktemp -d)/deadcode"
trap 'rm -rf "$(dirname "$tool")"' EXIT
go build -o "$tool" golang.org/x/tools/cmd/deadcode

found="$(mktemp)"
trap 'rm -rf "$(dirname "$tool")" "$found"' EXIT

first=1
for os in linux darwin windows; do
  # テスト専用の補助や interface 実装を除外するため -test を付ける。
  # node_modules には別のユーザーの Go が入っている。
  reported="$(mktemp)"
  GOOS="$os" "$tool" -test ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | grep -v node_modules \
    | sed 's/:[0-9]*:[0-9]*: unreachable func: /\t/' \
    | sort > "$reported"
  if [ "$first" = 1 ]; then
    cp "$reported" "$found"
    first=0
  else
    comm -12 "$found" "$reported" > "$found.next"
    mv "$found.next" "$found"
  fi
  rm -f "$reported"
done

# 許容する到達不能シンボルには理由の記載を必須とする。
allowed="$(grep -v '^\s*#' scripts/ci/deadcode-allowed.tsv | grep -v '^\s*$' | cut -f1,2 | sort)"

unexpected="$(comm -23 "$found" <(printf '%s\n' "$allowed"))"
vanished="$(comm -13 "$found" <(printf '%s\n' "$allowed"))"

status=0
if [ -n "$unexpected" ]; then
  echo "参照ゼロの関数がある（3 OS すべてで到達不能）:" >&2
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

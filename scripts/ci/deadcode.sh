#!/usr/bin/env bash
#
# 使われていない関数を探す。次の 2 つを別々に報告する。
#
# - 参照ゼロの関数: テストからも本番からも届かない。
# - テストからしか届かない製品の関数: テストが呼ぶので残っているが、本番の入口
#   （main）からは届かない。テストが製品の関数を直接呼ぶ形のまま残ると、本番の
#   経路とテストが確かめている経路が別物になる。
#
# OS 固有の参照を考慮し、Linux、macOS、Windows のすべてで到達不能な関数だけを
# 数える。
set -euo pipefail

# **並べ方と比べ方を揃える。** sort はロケールの照合順で並べ、comm はバイトで
# 比べる。en_US.UTF-8 のような環境では両者が食い違い、comm は「並んでいない」と
# 言って止まる——実際、`internal/platform/windows/toolchain.go` が増えた日に
# 手元だけが赤くなった。CI のランナーは C ロケールなので、そこでは見えない。
export LC_ALL=C

cd "$(dirname "$0")/../.."

operating_systems="linux darwin windows"

# tool 自体は host 向けに建てる。`go tool` に GOOS を渡すと tool ごと
# その OS 向けに建ててしまい、走らせられない。
tool="$(mktemp -d)/deadcode"
trap 'rm -rf "$(dirname "$tool")"' EXIT
go build -o "$tool" golang.org/x/tools/cmd/deadcode

work="$(mktemp -d)"
trap 'rm -rf "$(dirname "$tool")" "$work"' EXIT

# list_unreachable は、指定した OS で到達不能な関数を「ファイル<TAB>シンボル」で
# 並べる。残りの引数は deadcode へそのまま渡す（-test を付けるとテストも入口に数える）。
# node_modules には別のユーザーの Go が入っている。
list_unreachable() {
  local os="$1"
  shift
  GOOS="$os" "$tool" "$@" ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | grep -v node_modules \
    | sed 's/:[0-9]*:[0-9]*: unreachable func: /\t/' \
    | sort
}

# unreachable_everywhere は、そのファイルが建てられるすべての OS で到達不能な関数を
# 並べる。3 OS の報告の共通部分を取るだけでは、`_unix.go` や `_windows.go` にしか
# 無い関数が「他の OS の一覧に無い」という理由で共通部分から落ち、永久に検出され
# なかった。$1 は OS ごとの到達不能の一覧の接頭辞（<接頭辞>.<OS>）である。
unreachable_everywhere() {
  local reported="$1" file symbol os
  for os in $operating_systems; do
    cat "$reported.$os"
  done | sort -u | while IFS=$'\t' read -r file symbol; do
    for os in $operating_systems; do
      if grep -Fxq -- "$file" "$work/built.$os" && ! grep -Fxq -- "$file	$symbol" "$reported.$os"; then
        continue 2
      fi
    done
    printf '%s\t%s\n' "$file" "$symbol"
  done | sort
}

for os in $operating_systems; do
  # テスト専用の補助や interface 実装を「参照ゼロ」に数えないため -test を付ける。
  list_unreachable "$os" -test > "$work/unreachable-with-tests.$os"
  # 本番の入口（main）だけから辿る。
  list_unreachable "$os" > "$work/unreachable-from-main.$os"
  GOOS="$os" go list -f '{{$dir := .Dir}}{{range .GoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .TestGoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}' \
      ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | sed "s#^$(pwd)/##" \
    | sort > "$work/built.$os"
done

unreachable_everywhere "$work/unreachable-with-tests" > "$work/dead"
unreachable_everywhere "$work/unreachable-from-main" > "$work/unreachable-from-main"
# main から届かないもののうち、参照ゼロでないものは、テストからしか届かない。
comm -23 "$work/unreachable-from-main" "$work/dead" > "$work/test-only"

# check_allowed は、見つけた関数を許しの一覧と比べ、一覧に無いもの、一覧に
# あるのにもう見つからないもの、理由の無い行を報告し、1 つでもあれば 1 を返す。
# シンボルが `*` の行は、`/` で終わるディレクトリの package の関数をすべて許す
# （サブディレクトリは含まない）。
check_allowed() {
  local found="$1" allowed_file="$2" heading="$3" remedy="$4"
  local entries packages exact outside_packages unexpected vanished unexplained package status=0
  entries="$(grep -v '^\s*#' "$allowed_file" | grep -v '^\s*$' | cut -f1,2 | sort)"
  unexplained="$(grep -v '^\s*#' "$allowed_file" | grep -v '^\s*$' | awk -F'\t' 'NF < 3 || $3 ~ /^[[:space:]]*$/')"
  packages="$(printf '%s\n' "$entries" | awk -F'\t' '$2 == "*" { print $1 }')"
  exact="$(printf '%s\n' "$entries" | awk -F'\t' '$1 != "" && $2 != "*"')"

  # package ごと許したものを除いてから、関数ごとの許しと比べる。
  outside_packages="$(awk -F'\t' -v packages="$packages" '
    BEGIN { count = split(packages, list, "\n"); for (i = 1; i <= count; i++) allowed[list[i]] = 1 }
    { directory = $1; sub(/[^\/]*$/, "", directory); if (!(directory in allowed)) print }
  ' "$found")"
  unexpected="$(comm -23 <(printf '%s\n' "$outside_packages" | grep -v '^$' || true) <(printf '%s\n' "$exact"))"
  vanished="$(comm -13 "$found" <(printf '%s\n' "$exact") | grep -v '^$' || true)"
  for package in $packages; do
    if ! awk -F'\t' -v package="$package" '
      index($1, package) == 1 && substr($1, length(package) + 1) !~ /\// { matched = 1 }
      END { exit !matched }
    ' "$found"; then
      vanished="$(printf '%s\n%s\t*\n' "$vanished" "$package" | grep -v '^$')"
    fi
  done

  if [ -n "$unexpected" ]; then
    echo "$heading" >&2
    printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
    echo "$remedy" >&2
    status=1
  fi
  if [ -n "$vanished" ]; then
    echo "$allowed_file に、もう見つからないものが載っている:" >&2
    printf '%s\n' "$vanished" | sed 's/^/  /' >&2
    echo "$allowed_file から消すこと。" >&2
    status=1
  fi
  if [ -n "$unexplained" ]; then
    echo "$allowed_file に、理由の無い行がある:" >&2
    printf '%s\n' "$unexplained" | sed 's/^/  /' >&2
    status=1
  fi
  return "$status"
}

status=0
check_allowed "$work/dead" scripts/ci/deadcode-allowed.tsv \
  "参照ゼロの関数がある（建てられるすべての OS で到達不能）:" \
  "消すか、scripts/ci/deadcode-allowed.tsv に理由と一緒に書くこと。" || status=1
check_allowed "$work/test-only" scripts/ci/deadcode-test-only-allowed.tsv \
  "テストからしか届かない製品の関数がある（建てられるすべての OS で main から到達不能）:" \
  "本番の経路から使うか、テストのファイルへ移すか、scripts/ci/deadcode-test-only-allowed.tsv に理由と一緒に書くこと。" || status=1
if [ "$status" = 0 ]; then
  echo "deadcode: 参照ゼロの関数も、テストからしか届かない製品の関数も無い"
fi
exit "$status"

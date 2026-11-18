#!/usr/bin/env bash
#
# web・pages・demo の npm の依存を、high 以上の脆弱性について監査する。
#
# - **本番の依存（--omit=dev）は例外なしで監査する。** 画面やエンジンに入るものに、
#   許しの一覧は効かない。
# - 開発の依存も含めた監査では、scripts/ci/npm-audit-allowed.tsv に GHSA の番号・期限・
#   理由と一緒に書いた勧告だけを許す。修正版がまだ無く、ビルドやテストの道具にしか
#   入らない勧告のための逃げ道である。
#
# 一覧の行は、期限を過ぎたとき、もう見つからなくなったとき、理由が無いときにも失敗に
# する。修正版が出て依存を上げたら、その場で行を消すことになる。
set -euo pipefail
export LC_ALL=C

cd "$(dirname "$0")/../.."

allowed_file=scripts/ci/npm-audit-allowed.tsv
prefixes="web pages demo"
today="$(date -u +%F)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

status=0

# high 以上の勧告を「GHSA<TAB>package<TAB>題名」で並べる。勧告そのものは via の中の
# オブジェクトに入っている。文字列の via は「脆弱な版に依存している」という派生なので数えない。
list_advisories() {
  local prefix="$1" report="$work/audit.$1.json"
  # 見つかると npm audit は 0 以外で終わるので、終了コードではなく JSON を読む。
  npm audit --prefix "$prefix" --json > "$report" 2> "$work/audit.$prefix.err" || true
  if ! jq -e '.vulnerabilities' "$report" > /dev/null 2>&1; then
    echo "$prefix: npm audit の結果を読めない:" >&2
    cat "$report" "$work/audit.$prefix.err" | sed 's/^/  /' >&2
    return 1
  fi
  jq -r '
    .vulnerabilities[] | .via[] | objects
    | select(.severity == "high" or .severity == "critical")
    | [(.url | sub(".*/"; "")), .name, .title] | @tsv
  ' "$report" | sort -u
}

for prefix in $prefixes; do
  if ! npm audit --prefix "$prefix" --omit=dev --audit-level=high; then
    echo "$prefix: 本番の依存に high 以上の脆弱性がある。本番の依存は許しの一覧では見逃さない。" >&2
    status=1
  fi
  if ! list_advisories "$prefix" > "$work/found.$prefix"; then
    status=1
  fi
  sed "s/^/$prefix	/" "$work/found.$prefix" >> "$work/found"
done
touch "$work/found"

entries="$(grep -v '^\s*#' "$allowed_file" | grep -v '^\s*$' || true)"
printf '%s\n' "$entries" | cut -f1 | grep -v '^$' | sort -u > "$work/allowed-ids" || true

# 一覧に無い勧告。番号はファイルで渡す（macOS の awk は -v の値に改行を許さない）。
# NR == FNR で見分けると、許しが 0 件で 1 つ目のファイルが空のとき、全部を許してしまう。
unexpected="$(awk -F'\t' 'FILENAME == ARGV[1] { ok[$0] = 1; next } !($2 in ok) { print }' "$work/allowed-ids" "$work/found")"
if [ -n "$unexpected" ]; then
  echo "high 以上の脆弱性がある（<場所> <GHSA> <package> <題名>）:" >&2
  printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
  echo "依存を上げて直すこと。修正版が無く、ビルドやテストの道具にしか入らないなら、$allowed_file に期限と理由と一緒に書く。" >&2
  status=1
fi

# 一覧の行そのものを確かめる。
while IFS=$'\t' read -r id deadline reason; do
  [ -n "$id" ] || continue
  if [ -z "${reason// /}" ]; then
    echo "$allowed_file の $id に理由が無い。" >&2
    status=1
  fi
  if ! [[ "$deadline" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    echo "$allowed_file の $id の期限 '$deadline' が YYYY-MM-DD ではない。" >&2
    status=1
  elif [[ "$today" > "$deadline" ]]; then
    echo "$allowed_file の $id は期限 $deadline を過ぎた。修正版が出ていないか確かめ、直すか、期限を延ばす理由を書き直すこと。" >&2
    status=1
  fi
  if ! cut -f2 "$work/found" | grep -Fxq -- "$id"; then
    echo "$allowed_file の $id は、もうどこにも見つからない。$allowed_file から消すこと。" >&2
    status=1
  fi
done <<< "$entries"

if [ "$status" = 0 ]; then
  allowed_count="$(cut -f2 "$work/found" | sort -u | grep -c . || true)"
  echo "npm audit: 本番の依存に high 以上の脆弱性は無い。開発の依存で許しているのは ${allowed_count} 件（$allowed_file）。"
fi
exit "$status"

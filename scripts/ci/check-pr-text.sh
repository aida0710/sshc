#!/usr/bin/env bash
# PR のタイトル・本文と、ブランチのコミットメッセージを検査する。
#
#   PR_TITLE=… PR_BODY=… scripts/ci/check-pr-text.sh [<base> <head>]
#
# PR_TITLE があればタイトルを、PR_BODY があれば本文を、<base> と <head> を渡せば
# <base>..<head> のコミットメッセージを検査する。PR の CI（.github/workflows/pull-request-text.yml）は
# 3つとも、make check-commits はコミットメッセージだけを検査する。
#
# 決まりは3つある。
# - タイトルは `<type>(<scope>): <日本語の要約>`（scope は省略できる）。squash でマージすると、
#   タイトルが main のコミットの件名になる。
# - 本文とコミットメッセージに、AI ツールが追加する帰属の行を入れない。AI ツールはセッションのたびに
#   追加するよう指示されるので、書く側の注意だけでは防げない。リポジトリの設定で、squash の本文は
#   ブランチのコミットメッセージをつなげたものになるので、コミットメッセージもそのまま main に入る。
# - 本文とコミットメッセージに、社内の URL（ホストが .internal で終わるか、RFC 1918 の IP アドレス）を
#   書かない。公開リポジトリなので誰でも読める。画像は issue-assets ブランチに置いて貼る。
#
# 違反した文字列そのものは出力せず、決まりと場所（何行目か、どのコミットか）だけを出力する。
# 公開リポジトリの Actions のログは誰でも読めるので、消したい URL をログに残さないためである。
#
# 正規表現は Perl で評価する。grep は GNU・BSD・ugrep で -P や文字の種類の扱いが違い、この検査は
# Linux と macOS の Go のテストからも実行するためである。macOS の bash 3.2 でも動くように、配列と
# lastpipe は使わない。
set -euo pipefail

readonly title_types='feat fix docs test refactor perf build ci chore style revert'
# 要約には、ひらがな・カタカナ・漢字を1字以上含める。
readonly title_pattern='^(?:'"${title_types// /|}"')(?:\([^()\s]+\))?!?: (?=.*[\p{Hiragana}\p{Katakana}\p{Han}])\S'
# Claude Code の場合は、コミットメッセージの末尾の Claude-Session と Co-Authored-By の行、PR の本文の
# 末尾の Generated with の行と、セッションへのリンク。英語の文中の小文字の「generated with」は
# 対象にしない。
readonly attribution_pattern='(?i:^\s*(?:claude-session|co-authored-by):|claude\.ai/code/session_)|Generated with'
# .internal の後ろにホスト名が続くもの（.internal.example.com など）は、社内のホストとして扱わない。
readonly internal_url_pattern='(?i)https?://(?:[^\s/?#@]*@)?(?:(?:[a-z0-9-]+\.)+internal|10\.\d+\.\d+\.\d+|192\.168\.\d+\.\d+|172\.(?:1[6-9]|2\d|3[01])\.\d+\.\d+)(?![a-z0-9-]|\.[a-z0-9-])'

if [ $# -ne 0 ] && [ $# -ne 2 ]; then
  echo "usage: PR_TITLE=… PR_BODY=… $0 [<base> <head>]" >&2
  exit 2
fi

status=0
checked=''

# report <場所> <違反> は、違反した場所を出力し、終了の状態を失敗にする。
report() {
  printf '%s: %s\n' "$1" "$2" >&2
  status=1
}

# lines_matching <正規表現> <文> は、文のうち正規表現に一致する行の番号を、空白で区切って出力する。
lines_matching() {
  printf '%s\n' "$2" | RULE="$1" perl -CS -ne 'push @numbers, $. if /$ENV{RULE}/; END { print join(" ", @numbers) }'
}

# check_message <場所> <文> は、本文かコミットメッセージに、帰属の行と社内の URL が無いかを検査する。
check_message() {
  local place="$1" message="$2" lines
  lines="$(lines_matching "$attribution_pattern" "$message")"
  if [ -n "$lines" ]; then
    report "$place" "${lines} 行目に帰属の行（Co-Authored-By など）がある。AI ツールが追加するよう指示しても入れない"
  fi
  lines="$(lines_matching "$internal_url_pattern" "$message")"
  if [ -n "$lines" ]; then
    report "$place" "${lines} 行目に社内の URL（ホストが .internal で終わるか、RFC 1918 の IP アドレス）がある。画像は issue-assets ブランチに置いて貼る"
  fi
}

if [ -n "${PR_TITLE+set}" ]; then
  checked="${checked} タイトル"
  if [ -z "$(lines_matching "$title_pattern" "$PR_TITLE")" ]; then
    report "タイトル" "<type>(<scope>): <日本語の要約> の形ではない（例: fix(sftp): 一覧の並び順を修正する）。type は ${title_types// /・} のどれか。scope は省略できる"
  fi
fi

if [ -n "${PR_BODY+set}" ]; then
  checked="${checked} 本文"
  check_message "本文" "$PR_BODY"
fi

if [ $# -eq 2 ]; then
  # 代入にしておくと、範囲を読めなかったときに set -e で止まる。
  commits="$(git rev-list "$1..$2")"
  status_before_commits=$status
  count=0
  for commit in $commits; do
    count=$((count + 1))
    message="$(git log -1 --format=%B "$commit")"
    check_message "コミット $(git rev-parse --short "$commit") のメッセージ" "$message"
  done
  checked="${checked} コミット${count}件（$1..$2）"
  if [ "$status" -ne "$status_before_commits" ]; then
    echo "コミットメッセージは git commit --amend か git rebase で書き直し、push し直す" >&2
  fi
fi

if [ -z "$checked" ]; then
  echo "検査する対象が無い。PR_TITLE・PR_BODY か、<base> <head> を渡す" >&2
  exit 2
fi
if [ "$status" -eq 0 ]; then
  echo "違反は無かった:${checked}"
fi
exit "$status"

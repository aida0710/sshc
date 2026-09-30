#!/bin/sh
# 導入例に固定したバージョンが、指定したtagと一致するかを確かめる。
#
#   scripts/release/check-pinned-installers.sh v0.41.0
#
# 導入例を置く文書の一覧はこのscriptだけが持つ。publish.shとRelease workflowは公開する
# tagで、internal/buildcontractの契約テストはmainにある最新の安定版のリリースノートの
# バージョンで、このscriptを実行する。導入例を置く文書を足したら、ここの一覧を直す。
set -eu

die() { printf 'check-pinned-installers: %s\n' "$*" >&2; exit 1; }

[ "$#" -eq 1 ] || die "usage: $0 <tag>"
tag=$1
cd "$(dirname "$0")/../.."

# require_text は、documentがtextを含まなければ、どの例が古いかを示して終了する。
require_text() {
  document=$1
  text=$2
  grep -F -- "$text" "$document" >/dev/null || die "$document does not pin the installer to $tag: missing $text"
}

for document in README.md docs/release-install.md install.sh pages/guide/install.md pages/en/guide/install.md; do
  # mainのinstall.shはcommitごとに中身が変わるため、導入例から直接実行させない。
  if grep -F 'raw.githubusercontent.com/aida0710/sshc/main/install.sh' "$document" >/dev/null; then
    die "$document runs install.sh from main, whose content changes with every commit"
  fi
  require_text "$document" "SSHC_VERSION=$tag"
  require_text "$document" "/sshc/$tag/install.sh"
done
require_text docs/release-install.md "\$env:SSHC_VERSION = '$tag'"
require_text docs/release-install.md "releases/download/$tag/install.ps1"

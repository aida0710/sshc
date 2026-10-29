#!/bin/sh
# 画面ごとに、フォームファクタの画像を同じ高さにそろえて横に並べた1枚を作る（目で見比べるため）。
# 使い方: sh scripts/ux-audit/montage.sh <撮影の出力先> <並べた画像の出力先> <フォームファクタ...>
set -eu
shots=$(cd "$1" && pwd)
mkdir -p "$2"
sheets=$(cd "$2" && pwd)
shift 2
factors="$*"
screens=$(ls "$shots" | grep -- '--.*\.png$' | sed 's/^.*--//' | sort -u)
# 同じ screen 名の画像を、指定の順に並べる。無いものは飛ばす。
for screen in $screens; do
  inputs=""
  for factor in $factors; do
    file="$factor--$screen"
    [ -f "$shots/$file" ] && inputs="$inputs /shots/$file"
  done
  [ -z "$inputs" ] && continue
  docker run --rm -u "$(id -u):$(id -g)" -v "$shots:/shots:ro" -v "$sheets:/sheets" sshc-ux-audit-sshd:2026-10-30 \
    sh -c "magick $inputs -resize x900 -bordercolor '#888' -border 2 +append -resize '3200x900>' /sheets/$screen" 
  echo "$screen"
done

#!/usr/bin/env bash
# PR で変わったパスを標準入力から 1 行ずつ受け取り、Windows と macOS の Go のテストが
# 要るかを true／false で出力する。
#
# 画面（web/）、文書（pages/）、組み立てた画面のファイル（internal/ui/dist/）、確認用の
# 画像（artifacts/）、Markdown だけの変更なら false。それ以外のパスが 1 つでもあれば true。
# 迷うパス（.github/、scripts/、Makefile、go.mod など）と、パスを 1 つも受け取れなかったときは
# true の側に倒す。Linux の Go の
# テストはいつも走るので、web/src や pages を読む受け入れテストはそちらで確かめる。
set -eu

needed=false
seen=0
while IFS= read -r path; do
  [ -n "$path" ] || continue
  seen=$((seen + 1))
  case "$path" in
    web/*|pages/*|internal/ui/dist/*|artifacts/*|*.md) ;;
    *)
      needed=true
      echo "native Go tests needed for: $path" >&2
      ;;
  esac
done
if [ "$seen" -eq 0 ]; then
  echo "no changed paths were given; running the native Go tests" >&2
  needed=true
fi
echo "$needed"

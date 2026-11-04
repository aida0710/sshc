#!/usr/bin/env bash
# PR で変わったパスを標準入力から 1 行ずつ受け取り、ブラウザデモの確認（CI の jobs.browser-demo）が要るかを
# true／false で出力する。
#
# この確認は、画面とエンジンを組み立てて実際の VM で動かすので、画面（web/）、デモ（demo/）、エンジン（Go）、
# この判定と workflow 自身のどれかが変わったときだけ true にする。文書や Android だけの変更では動かさない。
# パスを 1 つも受け取れなかったときは true の側に倒す。
set -eu

needed=false
seen=0
while IFS= read -r path; do
  [ -n "$path" ] || continue
  seen=$((seen + 1))
  case "$path" in
    web/*|demo/*|cmd/*|internal/*|go.mod|go.sum|.github/workflows/*|scripts/ci/browser-demo-needed.sh)
      needed=true
      echo "browser demo check needed for: $path" >&2
      ;;
  esac
done
if [ "$seen" -eq 0 ]; then
  echo "no changed paths were given; running the browser demo check" >&2
  needed=true
fi
echo "$needed"

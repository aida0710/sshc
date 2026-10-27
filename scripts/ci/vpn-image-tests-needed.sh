#!/usr/bin/env bash
# 変わったパスを標準入力から 1 行ずつ受け取り、VPN のコンテナのイメージを組み立てて
# 接続を確かめる検査（CI の jobs.vpn-image）が要るかを true／false で出力する。
#
# この検査は、internal/vpn のテストと、それが埋め込むコンテナ（internal/vpn/container）だけを
# 確かめる。そこで、internal/vpn のテストが読み込む Go のパッケージ（go list -deps -test）の
# ディレクトリにあるファイル、コンテナ、モジュールの定義、この判定と workflow 自身のどれかが
# 変わったときだけ true にする。イメージは apt の snapshot から組み立てるので、関係の無い変更で
# 毎回組み立てると、時間がかかるうえに外部の障害で CI 全体が落ちる。
# パスを 1 つも受け取れなかったとき、読み込むパッケージを調べられなかったときは true の側に倒す。
set -eu

# 先に入力を全部読む。途中でやめると、書き手（git diff）が SIGPIPE で落ち、pipefail の下では
# 判定ごと失敗する。
paths=$(grep -v '^$' || true)
if [ -z "$paths" ]; then
  echo "no changed paths were given; running the VPN image check" >&2
  echo true
  exit 0
fi

repository=$(cd "$(dirname "$0")/../.." && pwd)
if ! listed=$(go list -C "$repository" -deps -test -f '{{if not .Standard}}{{.Dir}}{{end}}' ./internal/vpn); then
  echo "could not list the packages the VPN tests load; running the VPN image check" >&2
  echo true
  exit 0
fi
dependencies=$(printf '%s\n' "$listed" | sed -n "s|^$repository/||p" | sort -u)

needed=false
while IFS= read -r path; do
  relevant=false
  case "$path" in
    go.mod|go.sum|.github/workflows/ci.yml|scripts/ci/vpn-image-tests-needed.sh|internal/vpn/container/*)
      relevant=true
      ;;
    *)
      if printf '%s\n' "$dependencies" | grep -qxF "$(dirname "$path")"; then
        relevant=true
      fi
      ;;
  esac
  if [ "$relevant" = true ]; then
    needed=true
    echo "the VPN image check is needed for: $path" >&2
  fi
done <<< "$paths"
echo "$needed"

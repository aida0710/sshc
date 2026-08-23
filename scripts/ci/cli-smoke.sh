#!/usr/bin/env bash
#
# 出荷するその実体を、起こして確かめる。
#
# **綴りは中身を保証しない。** verify-artifact-name が見るのはファイル名だけで、
# nativebuild/machine.go 自身が「`sshc-linux-arm64` という名前の amd64 バイナリは
# その検査を通る」と書いている。束の smoke を消したあと、リリースは
# `make test` → build → upload だけになり、**上げるバイナリを一度も起動しなかった。**
#
# ここが確かめるのは、開発機の go test では出ない類の壊れ方である:
#
#   - 版が入っていない（-X が外れていれば "dev" のまま出る）
#   - 画面が入っていない（go:embed が空でも build は通る）
#   - engine が起きない（受け口、handoff、状態ディレクトリ）
#
# **走らせられるのは host の arch だけである。** 別の arch のものは名前しか
# 確かめられない。それを黙って「確かめた」と言わない。
set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: cli-smoke.sh <dist-dir> <expected-version>" >&2
	exit 2
fi
dist=$1
expected=$2

goos=$(go env GOOS)
goarch=$(go env GOARCH)
suffix=""
[ "$goos" = "windows" ] && suffix=".exe"
binary="$dist/sshc-$goos-$goarch$suffix"

if [ ! -x "$binary" ]; then
	echo "no runnable artifact for this machine at $binary" >&2
	ls -la "$dist" >&2
	exit 1
fi

say() { printf '  %s\n' "$1"; }
echo "cli-smoke: $binary"

# ① 自分が何であるかを言えること。
version_line=$("$binary" version)
say "$version_line"
[ "$version_line" = "sshc $expected $goos/$goarch" ] || {
	echo "version line is $version_line, want \"sshc $expected $goos/$goarch\"" >&2
	exit 1
}

# ② engine が居ないときに、次に何をすればよいかを言えること。
#
# **綴りではなく行動を返す。** ここが `open ...: no such file or directory` を
# 返していた頃、入れた直後の人が最初に読むのがそれだった。
set +e
absent=$("$binary" status 2>&1)
absent_code=$?
set -e
[ "$absent_code" -ne 0 ] || { echo "status succeeded with no engine" >&2; exit 1; }
case "$absent" in
	*"sshc engine"*) say "no engine: $absent" ;;
	*) echo "the no-engine message does not say what to do: $absent" >&2; exit 1 ;;
esac

# ③ 起こして、答えて、畳めること。
home=$(mktemp -d)
export HOME="$home"
mkdir -p "$home/.ssh"

"$binary" engine >"$home/engine.log" 2>&1 &
enginePID=$!
trap 'kill "$enginePID" 2>/dev/null || true; rm -rf "$home"' EXIT

handoff="$home/.ssh/sshc/cli"
for _ in $(seq 1 100); do
	[ -f "$handoff" ] && break
	sleep 0.2
done
[ -f "$handoff" ] || { echo "the engine never published a handoff" >&2; cat "$home/engine.log" >&2; exit 1; }

status=$("$binary" status)
say "$(printf '%s' "$status" | tr '\n' ';')"
case "$status" in
	*"running (pid"*) ;;
	*) echo "status did not report a running engine: $status" >&2; exit 1 ;;
esac

# ④ **画面が入っていること。** go:embed の中身が空でもビルドは通る。
#    実際に入口を開いて、SPA の器が返ることを見る。
entrance=$("$binary" open)
page=$(curl --silent --show-error --fail --max-time 10 "$entrance")
case "$page" in
	*'<div id="root">'*) say "the bundled UI answered" ;;
	*) echo "the entrance did not return the app shell" >&2; exit 1 ;;
esac

kill "$enginePID"
wait "$enginePID" 2>/dev/null || true
echo "cli-smoke: ok"

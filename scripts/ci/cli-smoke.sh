#!/usr/bin/env bash
#
# リリース対象のバイナリを実行して検証する。
# ファイル名だけでは OS やアーキテクチャを保証できない。
#
# ここが確かめるのは、開発機の go test では出ない類の壊れ方である:
#
#   - 版が入っていない（-X が外れていれば "dev" のまま出る）
#   - 画面が入っていない（go:embed が空でも build は通る）
#   - engine が起きない（受け口、handoff、状態ディレクトリ）
#
# 実行検証はホストと同じアーキテクチャの成果物だけを対象とする。
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

# ① バージョン、OS、アーキテクチャを確認する。
version_line=$("$binary" version)
say "$version_line"
[ "$version_line" = "sshc $expected $goos/$goarch" ] || {
	echo "version line is $version_line, want \"sshc $expected $goos/$goarch\"" >&2
	exit 1
}

# 実環境を変更せず、info と engine が同じ一時 HOME だけを見るようにする。
home=$(mktemp -d)
export HOME="$home"
mkdir -p "$home/.ssh"
cat >"$home/.ssh/config" <<'EOF'
Host smoke-info
  HostName 192.0.2.10
  User smoke-user
  Port 22
EOF
enginePID=""
cleanup() {
	if [ -n "$enginePID" ]; then
		kill "$enginePID" 2>/dev/null || true
	fi
	rm -rf -- "$home"
}
trap cleanup EXIT

# ② info が engine なしで実効接続先を安定した JSON として返すことを確認する。
info=$("$binary" info smoke-info --json)
printf '%s\n' "$info" | jq -e '
  .schemaVersion == 1 and
  .alias == "smoke-info" and
  .destination.hostName == "192.0.2.10" and
  .destination.user == "smoke-user" and
  .destination.port == "22"
' >/dev/null
say "info resolved smoke-info without an engine"

# ③ engine が停止中の場合に復旧手順が表示されることを確認する。
set +e
absent=$("$binary" status 2>&1)
absent_code=$?
set -e
[ "$absent_code" -ne 0 ] || { echo "status succeeded with no engine" >&2; exit 1; }
case "$absent" in
	*"sshc engine"*) say "no engine: $absent" ;;
	*) echo "the no-engine message does not say what to do: $absent" >&2; exit 1 ;;
esac

# ④ 起動、応答、正常終了を確認する。
"$binary" engine >"$home/engine.log" 2>&1 &
enginePID=$!

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

# ⑤ 埋め込み UI を HTTP 経由で取得できることを確認する。
entrance=$("$binary" open)
page=$(curl --silent --show-error --fail --max-time 10 "$entrance")
case "$page" in
	*'<div id="root">'*) say "the bundled UI answered" ;;
	*) echo "the entrance did not return the app shell" >&2; exit 1 ;;
esac

kill "$enginePID"
wait "$enginePID" 2>/dev/null || true
enginePID=""
echo "cli-smoke: ok"

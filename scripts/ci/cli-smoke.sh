#!/usr/bin/env bash
#
# リリース対象のバイナリを実行して検証する。
# ファイル名だけでは OS やアーキテクチャを保証できない。
#
# ここが確かめるのは、開発機の go test では出ない類の壊れ方である:
#
#   - バージョンが入っていない（-X が外れていれば "dev" のまま出る）
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
smokeHome=$(mktemp -d)
run_sshc() { env HOME="$smokeHome" "$binary" "$@"; }
mkdir -p "$smokeHome/.ssh"
cat >"$smokeHome/.ssh/config" <<'EOF'
Host smoke-info
  HostName 192.0.2.10
  User smoke-user
  Port 22
EOF
cleanup() {
	if run_sshc stop; then
		rm -rf -- "$smokeHome"
	else
		echo "could not stop the smoke engine; state retained at $smokeHome" >&2
	fi
}
trap cleanup EXIT

# ② info が engine なしで実効接続先を安定した JSON として返すことを確認する。
info=$(run_sshc info smoke-info --json)
printf '%s\n' "$info" | jq -e '
  .schemaVersion == 1 and
  .success == true and
  .result.schemaVersion == 1 and
  .result.alias == "smoke-info" and
  .result.destination.hostName == "192.0.2.10" and
  .result.destination.user == "smoke-user" and
  .result.destination.port == "22"
' >/dev/null
say "info resolved smoke-info without an engine"

# ③ engine が停止中の場合に復旧手順が表示されることを確認する。
set +e
absent=$(run_sshc status 2>&1)
absent_code=$?
set -e
[ "$absent_code" -ne 0 ] || { echo "status succeeded with no engine" >&2; exit 1; }
case "$absent" in
	*"sshc engine"*) say "no engine: $absent" ;;
	*) echo "the no-engine message does not say what to do: $absent" >&2; exit 1 ;;
esac

# ④ 起動、応答、正常終了を確認する。
entrance=$(run_sshc open --no-browser)
handoff="$smokeHome/.ssh/sshc/cli"
[ -f "$handoff" ] || { echo "the engine never published a handoff" >&2; exit 1; }

status=$(run_sshc status)
say "$(printf '%s' "$status" | tr '\n' ';')"
case "$status" in
	*"running (pid"*) ;;
	*) echo "status did not report a running engine: $status" >&2; exit 1 ;;
esac

# ⑤ 埋め込み UI を HTTP 経由で取得できることを確認する。
page=$(curl --silent --show-error --fail --max-time 10 "$entrance")
case "$page" in
	*'<div id="root">'*) say "the bundled UI answered" ;;
	*) echo "the entrance did not return the app shell" >&2; exit 1 ;;
esac

run_sshc stop
if run_sshc status >/dev/null 2>&1; then
	echo "the engine is still running after stop" >&2
	exit 1
fi
echo "cli-smoke: ok"

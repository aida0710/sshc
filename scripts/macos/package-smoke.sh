#!/bin/sh
# 束を実際に開いて、中身と振る舞いを確かめる。
#
# **Windows の package-smoke.ps1 と対になるものである。** あちらはインストーラが
# 何をしたかを見るが、macOS に インストーラは無い——利用者が .dmg から
# Applications へ引くだけなので、確かめるのは「その束が何を持っているか」と
# 「持っているものが動くか」になる。
#
# **秘密は出さない。** handoff にはワンタイムの資格情報が入っている。ここが
# 報告するのは、在るかどうかと、誰が読めるかだけである。
set -eu

usage() {
	echo "usage: $0 --dmg <path> --architecture <arm64|x64> --work-root <dir>" >&2
	exit 2
}

dmg=""
architecture=""
work_root=""
while [ $# -gt 0 ]; do
	case "$1" in
	--dmg) dmg="${2:-}"; shift 2 ;;
	--architecture) architecture="${2:-}"; shift 2 ;;
	--work-root) work_root="${2:-}"; shift 2 ;;
	*) usage ;;
	esac
done
[ -n "$dmg" ] && [ -n "$architecture" ] && [ -n "$work_root" ] || usage
[ -f "$dmg" ] || { echo "package-smoke: no disk image at $dmg" >&2; exit 1; }
case "$architecture" in arm64 | x64) ;; *) echo "package-smoke: unsupported architecture $architecture" >&2; exit 1 ;; esac

ok() { echo "ok: $1"; }
fail() { echo "package-smoke: expected $1" >&2; exit 1; }

mkdir -p "$work_root"
mounted=""
cleanup() {
	# **開いたものは、どう終わっても閉じる。** 途中で落ちた検査が
	# マウントを残していくのは、確かめに来ただけのものが機械を汚すことである。
	[ -n "$mounted" ] && hdiutil detach "$mounted" -quiet >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

mounted="$work_root/mnt"
mkdir -p "$mounted"
hdiutil attach "$dmg" -nobrowse -readonly -mountpoint "$mounted" >/dev/null
ok "the disk image mounted"

app="$mounted/sshc.app"
[ -d "$app" ] || fail "an application bundle at sshc.app"
ok "the bundle is named sshc.app"

# **封と中身が食い違っていないこと。** electron-builder は Electron の実行体を
# 貰ってきて Info.plist も icon も resources も差し替えるので、前の署名を残した
# ままだとここで落ちる。v0.1.0 の arm64 が実際にそうで、「開発元を確認できません」
# を越えたあとに「壊れているため開けません」になる状態のまま配られた。
# **これは配布署名の検査ではない**（それは spctl の仕事で、署名を買うまで通らない）。
# 束が自分自身と辻褄が合っているか、だけを見る。
codesign --verify --strict "$app" 2>/dev/null || fail "a bundle whose signature matches its contents"
ok "the bundle signature matches its contents"

cli="$app/Contents/Resources/sshc"
[ -f "$cli" ] || fail "the bundled CLI at Contents/Resources/sshc"
ok "the CLI is inside the bundle"

# **束ごとに、その束のアーキテクチャの実体が入っていること。** 一つを使い回すと
# ここが食い違い、ビルドは通って配ってから壊れる。
want="arm64"
[ "$architecture" = "x64" ] && want="x86_64"
have="$(lipo -archs "$cli" 2>/dev/null || echo unknown)"
case " $have " in
*" $want "*) ok "the CLI is built for $want (found: $have)" ;;
*) fail "the CLI to be $want, found $have" ;;
esac

# **束の中の Electron も同じ側である。**
shell="$app/Contents/MacOS/sshc"
[ -x "$shell" ] || fail "an executable shell at Contents/MacOS/sshc"
ok "the shell is executable"

# **走らせた事実を、走る機械の証明にしない。** arm64 の Mac は Rosetta 2 で
# x86_64 の実体を走らせる。動いたのは本当だが、それは「x64 の Mac で動く」とは
# 別のことである。どちらで確かめたのかを、結果と一緒に言う。
host="$(uname -m)"
native="no"
{ [ "$host" = "arm64" ] && [ "$architecture" = "arm64" ]; } && native="yes"
{ [ "$host" = "x86_64" ] && [ "$architecture" = "x64" ]; } && native="yes"
if [ "$native" = "no" ]; then
	echo "note: this host is $host; the $architecture binary runs here only through translation"
fi

# ここから先は、束の中の CLI をそのまま動かす。**Applications へ写さない**
# ——利用者の機械に置いていくものを、この検査は作らない。
home="$work_root/home"
rm -rf "$home"
mkdir -p "$home"

"$cli" --help >"$work_root/help.txt" 2>&1 || fail "the bundled CLI to run"
grep -q "headless" "$work_root/help.txt" || fail "its help to name the headless owner"
ok "the bundled CLI runs and names the headless owner"

HOME="$home" "$cli" headless >"$work_root/engine.out" 2>"$work_root/engine.err" &
engine=$!
kill_engine() {
	kill "$engine" 2>/dev/null || true
	wait "$engine" 2>/dev/null || true
}
trap 'kill_engine; cleanup' EXIT INT TERM

handoff="$home/.ssh/sshc/cli"
deadline=$(( $(date +%s) + 60 ))
while [ ! -f "$handoff" ] && [ "$(date +%s)" -lt "$deadline" ]; do sleep 0.2; done
[ -f "$handoff" ] || fail "the engine to publish a handoff"
ok "the engine published a handoff"

# **中身は読まない。** 誰が読めるかだけを見る。
mode=$(stat -f "%Lp" "$handoff")
[ "$mode" = "600" ] || fail "the handoff to be 0600, found $mode"
ok "the handoff is readable only by its owner"
mode=$(stat -f "%Lp" "$home/.ssh/sshc")
[ "$mode" = "700" ] || fail "the state directory to be 0700, found $mode"
ok "the state directory is closed to everyone else"

HOME="$home" "$cli" vault status >"$work_root/status.txt" 2>&1 || fail "vault status to answer"
grep -q "engine:[[:space:]]*headless" "$work_root/status.txt" || fail "the owner to be headless"
ok "the owner is headless"

# **端末が持っているあいだ、外殻はそれを横取りしない。**
if HOME="$home" "$cli" >"$work_root/bare.out" 2>"$work_root/bare.err"; then
	fail "bare sshc to refuse while a headless owner holds the engine"
fi
grep -q "headless" "$work_root/bare.err" || fail "the refusal to name the headless owner"
ok "bare sshc refused to displace the headless owner"

kill_engine
trap cleanup EXIT INT TERM
if [ "$native" = "yes" ]; then
	echo "package-smoke: $architecture passed natively on $host"
else
	echo "package-smoke: $architecture passed under translation on $host; not evidence for a native $architecture machine"
fi

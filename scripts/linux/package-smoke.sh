#!/bin/sh
# デスクトップの束を開いて、中身と振る舞いを確かめる。
#
# **Linux は形が二つある。** x64 は AppImage、arm64 は tar.gz である。
# electron-builder が arm64 向けに積む AppImage の runtime は版の付かない
# libz.so を要求し、普通の機械では起動しない——だから arm64 だけ形を変えた。
# 詳細は docs/manual-test-matrix.md にある。
#
# **macOS と Windows の package-smoke と対になる。** Linux にインストーラは
# 無く、利用者が AppImage をどこかへ置いて実行するだけなので、確かめるのは
# 「その束が何を持っているか」と「持っているものが動くか」になる。
#
# **`--appimage-extract` を使う。** 実行して中を見るには FUSE が要り、
# コンテナや素の CI ランナーには無い。展開なら FUSE を必要とせず、しかも
# 見るものは同じ——利用者が実行したときに開かれるのと同じ木である。
#
# **ただし展開も runtime を「実行」する。** だから走らせられないアーキの束は
# --appimage-extract では開けない。そこは squashfs を直接読む——AppImage は
# ELF の後ろに squashfs を繋いだものなので、ELF の終端が中身の開始位置になる。
# x64 の CI から arm64 の束の中身を見られるのは、この道だけである。
#
# **秘密は出さない。** handoff にはワンタイムの資格情報が入っている。ここが
# 報告するのは、在るかどうかと、誰が読めるかだけである。
set -eu

usage() {
	echo "usage: $0 --package <path> --architecture <x64|arm64> --work-root <dir>" >&2
	exit 2
}

package=""
architecture=""
work_root=""
while [ $# -gt 0 ]; do
	case "$1" in
	--package) package="${2:-}"; shift 2 ;;
	--architecture) architecture="${2:-}"; shift 2 ;;
	--work-root) work_root="${2:-}"; shift 2 ;;
	*) usage ;;
	esac
done
[ -n "$package" ] && [ -n "$architecture" ] && [ -n "$work_root" ] || usage
[ -f "$package" ] || { echo "package-smoke: no package at $package" >&2; exit 1; }
case "$architecture" in x64 | arm64) ;; *) echo "package-smoke: unsupported architecture $architecture" >&2; exit 1 ;; esac

ok() { echo "ok: $1"; }
fail() { echo "package-smoke: expected $1" >&2; exit 1; }

rm -rf "$work_root"
mkdir -p "$work_root"
image="$(cd "$(dirname "$package")" && pwd)/$(basename "$package")"

# **走らせられるかどうかを、開ける前に決める。** ここを展開の後ろに置いていた
# ときは、foreign-arch の束が "Exec format error" で落ち、「実行しなかった」と
# 言う分岐には決して到達しなかった。**書いたのに一度も走らない道だった。**
host="$(uname -m)"
native="no"
{ [ "$host" = "x86_64" ] && [ "$architecture" = "x64" ]; } && native="yes"
{ [ "$host" = "aarch64" ] && [ "$architecture" = "arm64" ]; } && native="yes"

# 束を開いて、中身の根を $root に置く。**形ごとに開け方が違う。**
case "$image" in
*.tar.gz)
	# **runtime を持たない形。** 開けるのに実行が要らないので、どの機械からでも
	# 同じように読める。arm64 の Linux をこの形で配っているのは、
	# electron-builder が arm64 向けに積む AppImage の runtime が、版の付かない
	# libz.so を要求して普通の機械では起動しないからである（x64 の runtime は
	# libz.so.1 を見ており無事）。
	tar xzf "$image" -C "$work_root" || fail "the archive to extract"
	root="$(find "$work_root" -mindepth 1 -maxdepth 1 -type d | head -1)"
	[ -n "$root" ] || fail "one directory at the root of the archive"
	ok "the archive extracted"
	;;
*.AppImage)
	if [ "$native" = "yes" ]; then
		# 利用者と同じ開け方をする。展開はカレントに squashfs-root を作るので、
		# 作業場所の中で行う。
		chmod +x "$image"
		if ! (cd "$work_root" && "$image" --appimage-extract >/dev/null 2>"$work_root/extract.err"); then
			# **理由を名指しする。** ここで一番起きるのは、runtime が要求する
			# 共有ライブラリが素の機械に無いことである。素のローダーの
			# メッセージだけを出すと、読んだ人は AppImage が壊れているのか
			# 環境が足りないのかを判別できない。
			if grep -q "libz.so:" "$work_root/extract.err" 2>/dev/null; then
				echo "package-smoke: this AppImage's runtime needs the unversioned libz.so," >&2
				echo "  which comes from zlib1g-dev and is absent from an ordinary system." >&2
				echo "  A user would not be able to start it." >&2
			fi
			cat "$work_root/extract.err" >&2
			fail "the AppImage to extract"
		fi
		root="$work_root/squashfs-root"
		[ -d "$root" ] || fail "the AppImage to extract"
		ok "the AppImage extracted"
	else
		# 実行せずに中身へ届く道。無ければ、確かめられなかったとだけ言う。
		if ! command -v unsquashfs >/dev/null 2>&1; then
			echo "note: this host is $host and cannot execute a $architecture runtime"
			echo "package-smoke: $architecture not inspected on $host; install squashfs-tools to read it here"
			exit 0
		fi
		# squashfs の開始位置 = ELF の終端 = 節表の位置 + 節の大きさ × 個数。
		offset="$(readelf -h "$image" 2>/dev/null | awk '
			/Start of section headers/ {start=$5}
			/Size of section headers/  {size=$5}
			/Number of section headers/{count=$5}
			END{ if (start != "" && size != "" && count != "") print start + size * count }')"
		[ -n "$offset" ] || fail "to read the ELF headers of the AppImage (is readelf present?)"
		root="$work_root/squashfs-root"
		unsquashfs -o "$offset" -d "$root" -q "$image" >"$work_root/extract.err" 2>&1 ||
			{ cat "$work_root/extract.err" >&2; fail "the squashfs inside the AppImage to be readable"; }
		ok "the AppImage was read without executing it"
	fi
	;;
*)
	echo "package-smoke: $image is neither an AppImage nor a tar.gz" >&2
	exit 1
	;;
esac

shell="$root/sshc"
[ -x "$shell" ] || fail "an executable shell at the root of the bundle"
ok "the shell is executable"

cli="$root/resources/sshc"
[ -f "$cli" ] || fail "the bundled CLI at resources/sshc"
chmod +x "$cli"
ok "the CLI is inside the bundle"

# **束ごとに、その束のアーキテクチャの実体が入っていること。** 一つを使い回すと
# ここが食い違い、ビルドは通って配ってから壊れる。
want="x86-64"
[ "$architecture" = "arm64" ] && want="aarch64"
# readelf は "AArch64" と綴る。**綴り方を当てにしない** —— 大文字小文字を
# 揃えてから照合する。
have="$(readelf -h "$cli" 2>/dev/null | sed -n 's/^ *Machine: *//p' | tr 'A-Z' 'a-z')"
case "$have" in
*"$want"*) ok "the CLI is built for $want (found: $have)" ;;
*) fail "the CLI to be $want, found ${have:-unreadable}" ;;
esac

if [ "$native" = "no" ]; then
	# **確かめた範囲を、通過と同じ言葉で言わない。**
	echo "package-smoke: $architecture contents checked on $host; nothing was executed"
	exit 0
fi

home="$work_root/home"
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
trap kill_engine EXIT INT TERM

handoff="$home/.ssh/sshc/cli"
deadline=$(( $(date +%s) + 60 ))
while [ ! -f "$handoff" ] && [ "$(date +%s)" -lt "$deadline" ]; do sleep 0.2; done
[ -f "$handoff" ] || fail "the engine to publish a handoff"
ok "the engine published a handoff"

# **中身は読まない。** 誰が読めるかだけを見る。
mode=$(stat -c "%a" "$handoff")
[ "$mode" = "600" ] || fail "the handoff to be 0600, found $mode"
ok "the handoff is readable only by its owner"
mode=$(stat -c "%a" "$home/.ssh/sshc")
[ "$mode" = "700" ] || fail "the state directory to be 0700, found $mode"
ok "the state directory is closed to everyone else"

HOME="$home" "$cli" vault status >"$work_root/status.txt" 2>&1 || fail "vault status to answer"
grep -q "engine:[[:space:]]*headless" "$work_root/status.txt" || fail "the owner to be headless"
ok "the owner is headless"

# **端末が持っているあいだ、外殻はそれを横取りしない。** 画面の無い機械では、
# 裸の `sshc` は窓の話をせずに headless を案内する。
if HOME="$home" DISPLAY= WAYLAND_DISPLAY= "$cli" >"$work_root/bare.out" 2>"$work_root/bare.err"; then
	fail "bare sshc to refuse while a headless owner holds the engine"
fi
grep -q "headless" "$work_root/bare.err" || fail "the refusal to name the headless owner"
ok "bare sshc refused to displace the headless owner"

kill_engine
trap - EXIT INT TERM
echo "package-smoke: $architecture passed natively on $host"

#!/bin/sh
# sshc の CLI を入れる。
#
#   SSHC_VERSION=v0.17.5 sh -c \
#     'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.17.5/install.sh | sh'
#
# 配布物は UI を埋め込んだ単一の CLI バイナリである。
# インストール前の検証内容と変更内容は標準出力へ表示する:
#
#   1. OS とアーキテクチャに対応する成果物の有無
#   2. ダウンロードした成果物の SHA-256
#   3. インストール先が PATH に含まれるか
#   4. 既存ファイルを安全に置換できるか
#   5. PATH 上で別の sshc が先に解決されないか
#   6. 稼働中の engine とインストール対象のバージョンが一致するか
#
# 環境変数:
#   SSHC_VERSION      入れる版（既定: 最新）
#   SSHC_INSTALL_DIR  置き先（既定: root なら /usr/local/bin、他は ~/.local/bin）

set -eu

REPO="aida0710/sshc"
say() { printf '%s\n' "$*"; }
note() { printf '  %s\n' "$*"; }
die() { printf 'sshc: %s\n' "$*" >&2; exit 1; }

# ── ダウンロードツール ─────────────────────────────────────────
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --connect-timeout 10 --max-time 180 "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -T 30 -t 3 -O "$2" "$1"; }
else
  die "neither curl nor wget is available"
fi

# ── ① 対応する成果物を選択する ─────────────────────────────────
# 未対応の OS またはアーキテクチャでは、代替成果物を推測せず終了する。
os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Linux) goos=linux ;;
  Darwin) goos=darwin ;;
  *) die "$os is not one of the systems this script installs (Linux, macOS). On Windows, download sshc-windows-amd64.exe or sshc-windows-arm64.exe from https://github.com/$REPO/releases/latest, rename it to sshc.exe, and place it on PATH." ;;
esac
case "$arch" in
  x86_64 | amd64) goarch=amd64 ;;
  aarch64 | arm64) goarch=arm64 ;;
  *) die "$arch is not an architecture sshc publishes a binary for" ;;
esac
asset="sshc-$goos-$goarch"

# ── 版を決める ────────────────────────────────────────────────
if [ -n "${SSHC_VERSION:-}" ]; then
  tag="$SSHC_VERSION"
  case "$tag" in v*) ;; *) tag="v$tag" ;; esac
  if ! printf '%s\n' "$tag" |
    grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'; then
    die "SSHC_VERSION is not a semantic version: $tag"
  fi
  base="https://github.com/$REPO/releases/download/$tag"
else
  tag=latest
  base="https://github.com/$REPO/releases/latest/download"
fi
say "sshc: installing $asset ($tag)"

# ── 置き先を決める ────────────────────────────────────────────
# root では /usr/local/bin、それ以外では sudo を要求せず ~/.local/bin を使用する。
if [ -n "${SSHC_INSTALL_DIR:-}" ]; then
  dir="$SSHC_INSTALL_DIR"
  why="SSHC_INSTALL_DIR"
elif [ "$(id -u)" = "0" ]; then
  dir="/usr/local/bin"
  why="running as root"
else
  dir="$HOME/.local/bin"
  why="not running as root, so nothing outside your home is touched"
fi
target="$dir/sshc"
note "into $target ($why)"

# ── ④ 既存ファイルを検査する ───────────────────────────────────
# シンボリックリンクは別の管理元を示す可能性があるため置換しない。
if [ -e "$target" ] || [ -L "$target" ]; then
  if [ -L "$target" ]; then
    die "$target is a symlink to $(readlink "$target"). Remove it first, or set SSHC_INSTALL_DIR."
  fi
  # rename の可否は対象ファイルではなく親ディレクトリの権限で決まる。
  [ -w "$dir" ] || die "$target exists and $dir is not writable by you. Re-run with sudo, or set SSHC_INSTALL_DIR."
  note "replacing the sshc already at $target"
fi

mkdir -p "$dir" || die "could not create $dir"
[ -w "$dir" ] || die "$dir is not writable by you. Re-run with sudo, or set SSHC_INSTALL_DIR."

# ── 落とす ────────────────────────────────────────────────────
work=$(mktemp -d) || die "could not create a temporary directory"
staged=""
receipt_staged=""
cleanup() {
  rm -rf "$work"
  [ -z "$staged" ] || rm -f "$staged"
  [ -z "$receipt_staged" ] || rm -f "$receipt_staged"
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT TERM
fetch "$base/$asset" "$work/sshc" || die "could not download $base/$asset"

# ── ② 公開された checksum と一致するか ────────────────────────
# 公開された checksums.txt と一致しない成果物はインストールしない。
if fetch "$base/checksums.txt" "$work/checksums.txt" 2>/dev/null; then
  expected=$(grep " $asset\$" "$work/checksums.txt" | cut -d' ' -f1 || true)
  [ -n "$expected" ] || die "checksums.txt does not list $asset"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$work/sshc" | cut -d' ' -f1)
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$work/sshc" | cut -d' ' -f1)
  else
    die "neither sha256sum nor shasum is available, so the download cannot be verified"
  fi
  [ "$actual" = "$expected" ] || die "the download does not match its published checksum
  expected $expected
  got      $actual"
  note "checksum matches"
else
  die "could not download $base/checksums.txt, so the download cannot be verified"
fi

chmod 0755 "$work/sshc"

# ── ⑥ 走っている engine と同じ版か ────────────────────────────
# 稼働中の engine と新しい CLI のバージョンが異なる場合は事前に通知する。
running=""
if command -v sshc >/dev/null 2>&1; then
  running=$(sshc status --json 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)
fi
incoming=$("$work/sshc" version 2>/dev/null | cut -d' ' -f2 || true)
[ -n "$incoming" ] || die "the downloaded sshc does not report its version"
if [ -n "$running" ] && [ -n "$incoming" ] && [ "$running" != "$incoming" ]; then
  note "an engine is running version $running; after this it will not match $incoming"
  note "quit the sshc app (or the terminal running it) and start it again"
fi

# ── 置く ──────────────────────────────────────────────────────
# target と同じディレクトリに完全な一時ファイルを作り、同一filesystem内の rename
# だけで公開する。/tmp からの mv はfilesystemをまたぐとcopyになり、既存targetを
# 部分的に書き換え得るため使用しない。
staged=$(mktemp "$dir/.sshc.install.XXXXXX") || die "could not stage the executable in $dir"
cp "$work/sshc" "$staged" && chmod 0755 "$staged" || die "could not stage the executable in $dir"

# install.sh由来であることをpathの推測に頼らず判定できるよう、実際に配置する
# binaryのdigestと版をreceiptへ結び付ける。receiptも同じdirectoryで原子的に公開する。
receipt="$dir/.sshc-install-receipt.json"
receipt_staged=$(mktemp "$dir/.sshc.receipt.XXXXXX") || die "could not stage the install receipt in $dir"
printf '{"schemaVersion":1,"manager":"install.sh","repository":"%s","version":"%s","sha256":"%s"}\n' \
  "$REPO" "$incoming" "$actual" > "$receipt_staged" || die "could not write the install receipt"
chmod 0644 "$receipt_staged" || die "could not protect the install receipt"
mv "$receipt_staged" "$receipt" || die "could not install the receipt into $receipt"
receipt_staged=""
mv "$staged" "$target" || die "could not install into $target"
staged=""

# version サブコマンドのない旧バージョンではインストール先だけを表示する。
installed=$("$target" version 2>/dev/null || true)
if [ -n "$installed" ]; then
  say "sshc: installed $installed"
else
  say "sshc: installed $target"
fi

# ── ⑤ PATH の手前に別の sshc が居ないか ───────────────────────
found=$(command -v sshc 2>/dev/null || true)
if [ -n "$found" ] && [ "$found" != "$target" ]; then
  say ""
  say "sshc: another sshc comes first on your PATH"
  note "$found runs when you type sshc"
  note "$target is the one this script just installed"
fi

# ── ③ 置き先が PATH に載っているか ────────────────────────────
# PATH は変更せず、必要な設定コマンドだけを表示する。
case ":$PATH:" in
  *":$dir:"*) exit 0 ;;
esac
say ""
say "sshc: $dir is not on your PATH, so typing sshc will not find it yet"
case "${SHELL##*/}" in
  zsh) rc="$HOME/.zshrc" ;;
  bash) rc="$HOME/.bashrc" ;;
  fish) note "fish_add_path $dir"; exit 0 ;;
  *) rc="your shell's startup file" ;;
esac
note "echo 'export PATH=\"$dir:\$PATH\"' >> $rc"
note "then open a new terminal"

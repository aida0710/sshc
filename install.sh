#!/bin/sh
# sshc の CLI を入れる。
#
#   curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
#
# **入れるのは CLI だけである。** デスクトップのアプリは dmg・AppImage・
# インストーラで配っており、あれは中に自分用の CLI を持っている。ここが置くのは
# 端末から `sshc` と打ったときに走るものひとつである。
#
# **この script は、置く前に確かめる。** curl から sh へ流し込むものが黙って
# 上書きするのは、渡された側からは何も見えない——だから、何を見て何を決めたかを
# すべて印字する。確かめるのは順に:
#
#   1. この機械のために作られた実体があるか（無ければ、何が無いのかを言う）
#   2. 落としたものが公開された checksum と一致するか
#   3. 置き先が PATH に載っているか（載っていなければ、足す 1 行を綴る）
#   4. 置き先に既に居るものが、この script の置いたものか
#   5. PATH の手前に別の sshc が居ないか
#   6. 走っている engine が、いま入れるものと同じ版か
#
# 環境変数:
#   SSHC_VERSION      入れる版（既定: 最新）
#   SSHC_INSTALL_DIR  置き先（既定: root なら /usr/local/bin、他は ~/.local/bin）

set -eu

REPO="aida0710/sshc"
say() { printf '%s\n' "$*"; }
note() { printf '  %s\n' "$*"; }
die() { printf 'sshc: %s\n' "$*" >&2; exit 1; }

# ── 落とす道具 ────────────────────────────────────────────────
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
  fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO "$2" "$1"; }
  fetch_stdout() { wget -qO- "$1"; }
else
  die "neither curl nor wget is available"
fi

# ── ① この機械のために作られた実体があるか ──────────────────
# **推測しない。** 知らない組み合わせに近そうなものを渡すより、何が無いのかを
# 言って止まる方が、動かない実体を PATH に置くより親切である。
os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Linux) goos=linux ;;
  Darwin) goos=darwin ;;
  *) die "$os is not one of the systems this script installs (Linux, macOS). Windows has an installer: https://github.com/$REPO/releases/latest" ;;
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
  base="https://github.com/$REPO/releases/download/$tag"
else
  tag=latest
  base="https://github.com/$REPO/releases/latest/download"
fi
say "sshc: installing $asset ($tag)"

# ── 置き先を決める ────────────────────────────────────────────
# **root で走っているなら /usr/local/bin である。** あそこは Linux でも macOS でも
# 既定の PATH に載っている。root でないなら ~/.local/bin へ置く——sudo を要求
# しないことの方が、PATH に載っていることより優先する（載っているかは下で確かめ、
# 載っていなければ足す 1 行を綴る）。
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

# ── ④ そこに既に居るものを見る ────────────────────────────────
# **自分が置いたもの以外は上書きしない。** そこに何を置くかは利用者が決めたこと
# であり、パッケージマネージャが持っているかもしれない。
if [ -e "$target" ] || [ -L "$target" ]; then
  if [ -L "$target" ]; then
    die "$target is a symlink to $(readlink "$target"). Something else manages it — remove it first, or set SSHC_INSTALL_DIR."
  fi
  # **書けるかどうかを決めるのはディレクトリである。** 読み取り専用のファイルでも、
  # 置いてあるディレクトリに書ければ rename で置き換えられる。
  [ -w "$dir" ] || die "$target exists and $dir is not writable by you. Re-run with sudo, or set SSHC_INSTALL_DIR."
  note "replacing the sshc already at $target"
fi

mkdir -p "$dir" || die "could not create $dir"
[ -w "$dir" ] || die "$dir is not writable by you. Re-run with sudo, or set SSHC_INSTALL_DIR."

# ── 落とす ────────────────────────────────────────────────────
work=$(mktemp -d) || die "could not create a temporary directory"
trap 'rm -rf "$work"' EXIT INT TERM
fetch "$base/$asset" "$work/sshc" || die "could not download $base/$asset"

# ── ② 公開された checksum と一致するか ────────────────────────
# **一致しないものは置かない。** curl から sh へ流し込む入れ方は、途中で
# すり替えられても受け取った側には見えない。checksums.txt は同じリリースが
# 公開しているので、少なくとも「落ちてきたものが、そのリリースのものか」は言える。
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
# **入れ替える前に訊く。** engine が走っている最中に実体を置き換えると、
# 端末の sshc と走っているアプリの版が食い違う。あちらはそれを検出して断るので、
# 壊れはしないが、理由が「なぜ今?」に見える。先に言っておく。
running=""
if command -v sshc >/dev/null 2>&1; then
  running=$(sshc status 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)
fi
incoming=$("$work/sshc" version 2>/dev/null | cut -d' ' -f2 || true)
if [ -n "$running" ] && [ -n "$incoming" ] && [ "$running" != "$incoming" ]; then
  note "an engine is running version $running; after this it will not match $incoming"
  note "quit the sshc app (or the terminal running it) and start it again"
fi

# ── 置く ──────────────────────────────────────────────────────
# **同じディレクトリへ書いてから rename する。** 半分書けたものがその名前を
# 持つ瞬間を作らない——実行される実体なので、その瞬間に起動した人は壊れた
# ファイルを実行する。
mv "$work/sshc" "$target" 2>/dev/null || {
  cp "$work/sshc" "$target.$$" && chmod 0755 "$target.$$" && mv "$target.$$" "$target"
} || die "could not install into $target"

# **答えられない実体もある。** 0.1.0 には version が無い——入ったことと、
# それが何かを言えることは別である。
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
# **PATH を勝手に書き換えない。** シェルの設定は利用者のものである。載って
# いないなら、足す 1 行をそのまま綴る——打つかどうかは向こうが決める。
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

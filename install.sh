#!/bin/sh
# sshc の CLI を入れる。
#
#   SSHC_VERSION=v0.42.0 sh -c \
#     'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.42.0/install.sh | sh'
#
# 配布物は UI を埋め込んだ単一の CLI バイナリである。
# インストール前の検証内容と変更内容は標準出力へ表示する:
#
#   1. OS とアーキテクチャに対応する成果物の有無（Rosetta 2 の下でも Mac の CPU で選ぶ）
#   2. ダウンロードした成果物の SHA-256 と、gh が使えるときは Release workflow の attestation
#   3. インストール先が PATH に含まれるか
#   4. 既存ファイルを安全に置換できるか（シンボリックリンクやディレクトリではないか）
#   5. PATH 上で別の sshc が先に解決されないか
#   6. 稼働中の engine とインストール対象のバージョンが一致するか
#   7. 取得物が名乗る OS・アーキテクチャ・バージョンが、選んだ成果物と一致するか
#
# 環境変数:
#   SSHC_VERSION      入れるバージョン（既定: 最新）
#   SSHC_INSTALL_DIR  置き先（既定: root なら /usr/local/bin、他は ~/.local/bin）
#   SSHC_INSTALL_CALLER  sshc update が "update" を渡す。利用者が設定するものではない

set -eu

REPO="aida0710/sshc"
say() { printf '%s\n' "$*"; }
note() { printf '  %s\n' "$*"; }
die() { printf 'sshc: %s\n' "$*" >&2; exit 1; }
# sshc updateとsshc service installがreceiptで受け付ける安定バージョン（prereleaseと
# build metadataのないSemVer）かを返す。
is_stable_version() {
  printf '%s\n' "$1" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
}
# regex_literal は、タグやリポジトリ名の . と + を、正規表現の記号ではなくその文字そのものとして照合させる。
regex_literal() {
  printf '%s' "$1" | sed 's/[.+]/\\&/g'
}

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
  *) die "$os is not one of the systems this script installs (Linux, macOS). On Windows, run: powershell -NoProfile -ExecutionPolicy Bypass -Command \"irm https://github.com/$REPO/releases/latest/download/install.ps1 | iex\"" ;;
esac
case "$arch" in
  x86_64 | amd64) goarch=amd64 ;;
  aarch64 | arm64) goarch=arm64 ;;
  *) die "$arch is not an architecture sshc publishes a binary for" ;;
esac
# Rosetta 2 の下で動くシェルでは uname -m が x86_64 を返す。Apple Silicon の Mac に
# amd64 版を入れないよう、CPU が arm64 を備えているかを sysctl で確かめる。
under_rosetta=false
if [ "$goos" = darwin ] && [ "$goarch" = amd64 ] &&
  [ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then
  goarch=arm64
  under_rosetta=true
fi
asset="sshc-$goos-$goarch"

# ── バージョンを決める ────────────────────────────────────────────
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
if [ "$under_rosetta" = true ]; then
  note "this shell runs under Rosetta 2, so the build for this Mac's arm64 CPU is installed"
fi

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
  if [ -d "$target" ]; then
    die "$target is a directory. Remove it first, or set SSHC_INSTALL_DIR."
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

# ── ② Release workflow が署名した成果物か ─────────────────────────
# checksums.txt は成果物と同じ Release から取るので、Release を書き換えられると
# 成果物と一緒に差し替えられる。GitHub CLI が github.com にログインしていれば、
# Release workflow が GitHub-hosted runner で署名した attestation も確かめる。
# 署名者は scripts/release/publish.sh と同じく、このタグの push で動いた Release workflow か、
# main から同じタグを作り直した Release workflow に限る。
# 最新版を入れるときはタグがまだ分からないので、どのタグの Release workflow でも受け付ける。
# gh attestation は gh 2.49.0 で入ったサブコマンドで、それより古い gh（Ubuntu 24.04 の apt が
# 入れる gh など）は unknown command で終わる。検証できない gh を署名の不一致と取り違えて
# 止めないよう、gh が無いときと同じくチェックサムだけで進める。
if ! command -v gh >/dev/null 2>&1; then
  note "GitHub CLI (gh) is not installed, so the release attestation was not checked; only the checksum was"
elif ! gh attestation verify --help >/dev/null 2>&1; then
  note "GitHub CLI (gh) cannot verify attestations (gh 2.49.0 or later is needed), so the release attestation was not checked; only the checksum was"
elif ! gh auth status --hostname github.com >/dev/null 2>&1; then
  note "GitHub CLI (gh) is not signed in to github.com, so the release attestation was not checked; only the checksum was"
else
  case "$tag" in
    latest) signed_tag='v[^/]+' ;;
    *) signed_tag=$(regex_literal "$tag") ;;
  esac
  signer="^https://github\\.com/$(regex_literal "$REPO")/\\.github/workflows/release\\.yml@refs/(tags/$signed_tag|heads/main)\$"
  # 止めるのは、検証が走って通らなかったときだけにする。署名の不一致とネットワークや API の
  # 失敗を読み分けられるよう、理由を決めつけず gh のエラーをそのまま見せる。
  if ! gh attestation verify "$work/sshc" --repo "$REPO" \
    --cert-identity-regex "$signer" --deny-self-hosted-runners >/dev/null 2>"$work/attestation.err"; then
    die "gh attestation verify did not confirm that the Release workflow of $REPO signed the download, so nothing was installed:
$(cat "$work/attestation.err")"
  fi
  note "attestation matches the Release workflow of $REPO"
fi

# ── 置き先に用意する ──────────────────────────────────────────
# target と同じディレクトリに完全な一時ファイルを作り、同一filesystem内の rename
# だけで公開する。/tmp からの mv はfilesystemをまたぐとcopyになり、既存targetを
# 部分的に書き換え得るため使用しない。
staged=$(mktemp "$dir/.sshc.install.XXXXXX") || die "could not stage the executable in $dir"
cp "$work/sshc" "$staged" && chmod 0755 "$staged" || die "could not stage the executable in $dir"

# ── ⑦ 取得物が選んだ成果物を名乗るか ─────────────────────────────
# 取得物は TMPDIR ではなく置き先で実行する。/tmp を noexec でマウントしたマシンでも
# 実行でき、実行できないときはシェルのエラー（Permission denied など）をそのまま見せる。
if ! version_line=$("$staged" version 2>"$work/version.err") || [ -z "$version_line" ]; then
  version_error=$(cat "$work/version.err")
  die "the downloaded sshc does not report its version${version_error:+: $version_error}"
fi
# 別の OS・アーキテクチャ・バージョンを名乗るものは、receipt に記録せず置かない。
incoming=$(printf '%s\n' "$version_line" | sed -n '1s/^sshc \(v[^ ]*\) .*$/\1/p')
if [ -z "$incoming" ] || [ "$version_line" != "sshc $incoming $goos/$goarch" ]; then
  die "the downloaded executable does not identify itself as sshc for $goos/$goarch: $version_line"
fi
if [ "$tag" != latest ] && [ "$incoming" != "$tag" ]; then
  die "the downloaded executable reports $incoming, expected $tag"
fi

# ── ⑥ 走っている engine と同じバージョンか ────────────────────────
# 稼働中の engine と新しい CLI のバージョンが異なる場合は事前に通知する。
running=""
if command -v sshc >/dev/null 2>&1; then
  running=$(sshc status --json 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)
fi
if [ -n "$running" ] && [ "$running" != "$incoming" ]; then
  note "an engine is running version $running; after this it will not match $incoming"
  # sshc update は、更新のあとで常駐サービスを自分で再起動し、ほかの engine の
  # 再起動の仕方も表示する。同じ案内を二度出さないよう、ここでは直接の実行だけに出す。
  if [ "${SSHC_INSTALL_CALLER:-}" != update ]; then
    note "to use $incoming, restart the managed service with \`sshc service install\`, or any other engine with \`sshc engine --replace\`"
  fi
fi

# ── 置く ──────────────────────────────────────────────────────
# install.sh由来であることをpathの推測に頼らず判定できるよう、実際に配置する
# binaryのdigestとバージョンをreceiptへ結び付ける。receiptも同じdirectoryで原子的に公開する。
# 2つのrenameの間で止まっても、receiptが別のbinaryを指さないよう、前のreceiptを先に消し、
# binaryを置いてから新しいreceiptを置く。途中で止まった導入はreceiptのない管理外の導入になり、
# このスクリプトを実行し直せば戻る。
# prereleaseは自動更新と常駐登録の対象外なので、新しいreceiptを書かない。
receipt="$dir/.sshc-install-receipt.json"
if is_stable_version "$incoming"; then
  receipt_staged=$(mktemp "$dir/.sshc.receipt.XXXXXX") || die "could not stage the install receipt in $dir"
  printf '{"schemaVersion":1,"manager":"install.sh","repository":"%s","version":"%s","sha256":"%s"}\n' \
    "$REPO" "$incoming" "$actual" > "$receipt_staged" || die "could not write the install receipt"
  chmod 0644 "$receipt_staged" || die "could not protect the install receipt"
fi
rm -f -- "$receipt" || die "could not remove the install receipt $receipt"
mv "$staged" "$target" || die "could not install into $target; run this script again"
staged=""
if [ -n "$receipt_staged" ]; then
  mv "$receipt_staged" "$receipt" || die "could not install the receipt into $receipt; run this script again"
  receipt_staged=""
fi

installed=$("$target" version 2>/dev/null || true)
[ -n "$installed" ] || die "the installed sshc at $target does not report its version"
say "sshc: installed $installed"
if ! is_stable_version "$incoming"; then
  note "$incoming is not a stable release, so sshc update and sshc service install do not manage this copy"
  note "to manage it again, install a stable version with this script"
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
shell_name=${SHELL:-}
case "${shell_name##*/}" in
  zsh) rc="$HOME/.zshrc" ;;
  bash) rc="$HOME/.bashrc" ;;
  fish) note "fish_add_path $dir"; exit 0 ;;
  *) rc="your shell's startup file" ;;
esac
note "echo 'export PATH=\"$dir:\$PATH\"' >> $rc"
note "then open a new terminal"

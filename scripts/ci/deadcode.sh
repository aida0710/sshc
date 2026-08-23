#!/usr/bin/env bash
#
# 参照ゼロの関数を探す。
#
# **一つの OS で見ても分からない。** `windowsacl.ValidatePrivatePath` を呼ぶのは
# `_windows.go` だけなので、Linux から見れば到達不能に見える。逆に Windows から
# 見れば `platform/macos` が丸ごと死んで見える。だから**出荷する 3 つの OS すべてで
# 到達不能なものだけ**を死んだものとして扱う——それが積である。
#
# **なぜ要るのか。** Electron と生体認証を消したとき、`windowsregistry` package
# （130 行）と `Vault.Wrap` は書き手が消えたまま残った。どちらも `//go:build windows`
# だったり本番から呼ばれないだけだったりで、コンパイルは通り続ける。**通るものは、
# 見張らなければ気づかれない。**
set -euo pipefail

cd "$(dirname "$0")/../.."

# tool 自体は host 向けに建てる。`go tool` に GOOS を渡すと tool ごと
# その OS 向けに建ててしまい、走らせられない。
tool="$(mktemp -d)/deadcode"
trap 'rm -rf "$(dirname "$tool")"' EXIT
go build -o "$tool" golang.org/x/tools/cmd/deadcode

found="$(mktemp)"
trap 'rm -rf "$(dirname "$tool")" "$found"' EXIT

first=1
for os in linux darwin windows; do
  # **-test を付ける。** テストからしか呼ばれない補助を「死んでいる」と言うのは
  # 行き過ぎで、interface を満たすためだけに在る stub まで巻き込む。
  # node_modules には他人の Go が入っている。
  reported="$(mktemp)"
  GOOS="$os" "$tool" -test ./cmd/... ./internal/... ./mobile/... 2>/dev/null \
    | grep -v node_modules \
    | sed 's/:[0-9]*:[0-9]*: unreachable func: /\t/' \
    | sort > "$reported"
  if [ "$first" = 1 ]; then
    cp "$reported" "$found"
    first=0
  else
    comm -12 "$found" "$reported" > "$found.next"
    mv "$found.next" "$found"
  fi
  rm -f "$reported"
done

# 許すものは理由と一緒に書いてある。**理由の無い許しは置かない。**
allowed="$(grep -v '^\s*#' scripts/ci/deadcode-allowed.tsv | grep -v '^\s*$' | cut -f1,2 | sort)"

unexpected="$(comm -23 "$found" <(printf '%s\n' "$allowed"))"
vanished="$(comm -13 "$found" <(printf '%s\n' "$allowed"))"

status=0
if [ -n "$unexpected" ]; then
  echo "参照ゼロの関数がある（3 OS すべてで到達不能）:" >&2
  printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
  echo "消すか、scripts/ci/deadcode-allowed.tsv に理由と一緒に書くこと。" >&2
  status=1
fi
if [ -n "$vanished" ]; then
  echo "許しの一覧に、もう死んでいないものが載っている:" >&2
  printf '%s\n' "$vanished" | sed 's/^/  /' >&2
  echo "scripts/ci/deadcode-allowed.tsv から消すこと。" >&2
  status=1
fi
[ "$status" = 0 ] && echo "deadcode: 参照ゼロの関数は無い"
exit "$status"

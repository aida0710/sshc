#!/usr/bin/env bash
#
# 前のリリース（GitHub の最新の安定版）のうち、このランナーで動く sshc を落とし、
# 同じリリースの checksums.txt と照らしてから、そのパスを出す。前のリリースから上げる
# 試験（cmd/sshc/upgrade_test.go）が使う。
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "usage: previous-release.sh <directory>" >&2
	exit 2
fi
directory=$1
repository=${GITHUB_REPOSITORY:-aida0710/sshc}

tag=$(gh release view --repo "$repository" --json tagName --jq .tagName)
goos=$(go env GOOS)
goarch=$(go env GOARCH)
suffix=""
[ "$goos" = "windows" ] && suffix=".exe"
asset="sshc-$goos-$goarch$suffix"

mkdir -p "$directory"
gh release download "$tag" --repo "$repository" --dir "$directory" \
	--pattern "$asset" --pattern checksums.txt --clobber
expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$directory/checksums.txt")
if [ -z "$expected" ]; then
	echo "checksums.txt of $tag has no line for $asset" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$directory/$asset" | awk '{ print $1 }')
else
	actual=$(shasum -a 256 "$directory/$asset" | awk '{ print $1 }')
fi
if [ "$actual" != "$expected" ]; then
	echo "$asset of $tag does not match checksums.txt" >&2
	exit 1
fi
chmod +x "$directory/$asset"
echo "previous release: $tag ($asset)" >&2
printf '%s\n' "$directory/$asset"

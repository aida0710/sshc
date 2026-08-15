#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: verify-artifact-name.sh ARTIFACT OS ARCHITECTURE" >&2
	exit 2
fi

artifact=$1
target_os=$2
architecture=$3

case "$target_os" in
	darwin|linux) suffix= ;;
	windows) suffix=.exe ;;
	*)
		echo "unsupported artifact OS: $target_os" >&2
		exit 2
		;;
esac

case "$architecture" in
	amd64|arm64) ;;
	*)
		echo "unsupported artifact architecture: $architecture" >&2
		exit 2
		;;
esac

name=${artifact##*/}
expected="sshc-$target_os-$architecture$suffix"
if [ "$name" != "$expected" ]; then
	echo "artifact name mismatch: expected $expected, got $name" >&2
	exit 1
fi

echo "verified artifact name: $name"

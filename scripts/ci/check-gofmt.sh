#!/usr/bin/env sh

set -eu

path_list="$(mktemp)"
unformatted="$(mktemp)"
trap 'rm -f "$path_list" "$unformatted"' 0 HUP INT TERM

git ls-files -z -- '*.go' >"$path_list"
if [ -s "$path_list" ]; then
	xargs -0 gofmt -l -- <"$path_list" >"$unformatted"
fi

if [ -s "$unformatted" ]; then
	printf '%s\n' 'These files are not gofmt-formatted. Run: gofmt -w <path>.' >&2
	cat "$unformatted" >&2
	exit 1
fi

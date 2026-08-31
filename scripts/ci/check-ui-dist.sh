#!/usr/bin/env sh

set -eu

repository="$(git rev-parse --show-toplevel)"
cd "$repository"

changes="$(git status --porcelain=v1 --untracked-files=all -- internal/ui/dist)"
if [ -n "$changes" ]; then
	printf '%s\n' 'Embedded UI assets differ from the Web production build. Run: make verify-ui-dist.' >&2
	printf '%s\n' "$changes" >&2
	exit 1
fi

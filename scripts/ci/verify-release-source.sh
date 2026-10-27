#!/usr/bin/env bash
# Release tagがsemantic versionで、そのcommitが保護対象のmain履歴にあるかを検証する。
# 同じcommitのmain CIの成功は、Releaseワークフローのmain-ci jobが公開の前に待つ。
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN is required}"
: "${RELEASE_REPOSITORY:?RELEASE_REPOSITORY is required}"
: "${RELEASE_SHA:?RELEASE_SHA is required}"
: "${RELEASE_TAG:?RELEASE_TAG is required}"

if ! printf '%s\n' "$RELEASE_TAG" |
  grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'; then
  echo "release tag is not semantic version: $RELEASE_TAG" >&2
  exit 1
fi

tag_sha=$(git rev-parse "$RELEASE_TAG^{commit}")
if [ "$tag_sha" != "$RELEASE_SHA" ]; then
  echo "release tag and workflow SHA differ" >&2
  exit 1
fi

git fetch --no-tags origin '+refs/heads/main:refs/remotes/origin/main'
if ! git merge-base --is-ancestor "$RELEASE_SHA" refs/remotes/origin/main; then
  echo "release SHA is not in origin/main history" >&2
  exit 1
fi

echo "release source verified: the tag is semantic and its commit is in main"

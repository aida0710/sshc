#!/usr/bin/env bash
# Release tagが保護対象のmain履歴にあり、同じcommitの完全なCIが成功済みかを検証する。
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

runs=$(gh api \
  -H 'Accept: application/vnd.github+json' \
  "repos/$RELEASE_REPOSITORY/actions/workflows/ci.yml/runs?head_sha=$RELEASE_SHA&status=completed&per_page=100")
if ! printf '%s' "$runs" | jq -e --arg sha "$RELEASE_SHA" '
  any(.workflow_runs[];
    .head_sha == $sha and
    .head_branch == "main" and
    .event == "push" and
    .conclusion == "success"
  )
' >/dev/null; then
  echo "the exact release SHA has no successful completed main push CI workflow" >&2
  exit 1
fi

echo "release source verified: main ancestor and exact-SHA CI succeeded"

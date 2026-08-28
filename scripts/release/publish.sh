#!/usr/bin/env bash
# mainの検証済みcommitをtag化し、保護gateの承認、公開待機、成果物検証まで行う。
set -Eeuo pipefail

repository=${SSHC_RELEASE_REPOSITORY:-aida0710/sshc}
tap_repository=${SSHC_HOMEBREW_TAP_REPOSITORY:-aida0710/homebrew-tap}
poll_seconds=${SSHC_RELEASE_POLL_SECONDS:-15}
mode=publish

usage() {
  cat <<'EOF'
Usage:
  scripts/release/publish.sh <vMAJOR.MINOR.PATCH[-PRERELEASE]>
  scripts/release/publish.sh --verify-only <tag>

publish:
  HEADとorigin/mainの一致、同じSHAのmain CI成功を確認してannotated tagをpushし、
  release environmentの保護gateを承認してGitHub Release完了まで待ちます。

--verify-only:
  既存Releaseのchecksum、attestation、実行版、APK、本文、Homebrew tapを検証します。
EOF
}

die() {
  printf 'release: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command is missing: $1"
}

if [ "${1:-}" = "--verify-only" ]; then
  mode=verify
  shift
fi
tag=${1:-}
[ -n "$tag" ] || { usage >&2; exit 2; }
[ "$#" -eq 1 ] || { usage >&2; exit 2; }

if ! printf '%s\n' "$tag" |
  grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'; then
  die "tag is not semantic version: $tag"
fi

for command in git gh jq curl unzip; do
  require_command "$command"
done
if command -v sha256sum >/dev/null 2>&1; then
  sha256_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  sha256_tool=shasum
else
  die 'sha256sum or shasum is required'
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || die 'run this inside the sshc repository'
cd "$repo_root"
gh auth status >/dev/null 2>&1 || die 'GitHub CLI is not authenticated'

sha256_value() {
  if [ "$sha256_tool" = sha256sum ]; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_checksum_file() {
  if [ "$sha256_tool" = sha256sum ]; then
    sha256sum -c checksums.txt
  else
    shasum -a 256 -c checksums.txt
  fi
}

verify_public_release() {
  local release expected_prerelease release_tmp formula source_archive
  local expected_assets actual_assets host_asset host_os host_arch version_output
  local formula_tag formula_sha actual_sha release_notes release_body

  printf 'release: verifying published %s\n' "$tag"
  release=$(gh api "repos/$repository/releases/tags/$tag") || die "published release not found: $tag"
  expected_prerelease=false
  case "$tag" in
    *-*|*+*) expected_prerelease=true ;;
  esac
  printf '%s' "$release" | jq -e \
    --arg tag "$tag" \
    --argjson prerelease "$expected_prerelease" '
      .tag_name == $tag and
      .draft == false and
      .prerelease == $prerelease and
      .immutable == true and
      all(.assets[]; .state == "uploaded" and .size > 0)
    ' >/dev/null || die 'release metadata is incomplete or mutable'

  expected_assets=$(printf '%s\n' \
    checksums.txt \
    "sshc-android-$tag.apk" \
    sshc-darwin-amd64 \
    sshc-darwin-arm64 \
    sshc-linux-amd64 \
    sshc-linux-arm64 \
    sshc-windows-amd64.exe \
    sshc-windows-arm64.exe | sort)
  actual_assets=$(printf '%s' "$release" | jq -r '.assets[].name' | sort)
  [ "$actual_assets" = "$expected_assets" ] || die 'release asset set differs from the publication contract'

  release_tmp=$(mktemp -d "${TMPDIR:-/tmp}/sshc-release-verify.XXXXXX")
  trap 'rm -rf -- "$release_tmp"' RETURN
  gh release download "$tag" --repo "$repository" --dir "$release_tmp"
  (
    cd "$release_tmp"
    verify_checksum_file
  )
  for artifact in "$release_tmp"/*; do
    gh attestation verify "$artifact" --repo "$repository" >/dev/null
    printf 'release: attestation OK: %s\n' "$(basename "$artifact")"
  done

  unzip -t "$release_tmp/sshc-android-$tag.apk" >/dev/null || die 'APK archive verification failed'
  host_os=$(uname -s)
  host_arch=$(uname -m)
  host_asset=
  case "$host_os/$host_arch" in
    Linux/x86_64) host_asset=sshc-linux-amd64 ;;
    Linux/aarch64|Linux/arm64) host_asset=sshc-linux-arm64 ;;
    Darwin/x86_64) host_asset=sshc-darwin-amd64 ;;
    Darwin/arm64) host_asset=sshc-darwin-arm64 ;;
  esac
  if [ -n "$host_asset" ]; then
    chmod +x "$release_tmp/$host_asset"
    version_output=$("$release_tmp/$host_asset" version)
    printf '%s\n' "$version_output"
    printf '%s\n' "$version_output" | grep -F "sshc $tag " >/dev/null || die 'native artifact reports the wrong version'
  else
    printf 'release: native smoke skipped on %s/%s\n' "$host_os" "$host_arch"
  fi

  release_notes="docs/releases/$tag.md"
  [ -f "$release_notes" ] || die "release notes are missing: $release_notes"
  release_body=$(printf '%s' "$release" | jq -r '.body')
  [ "$release_body" = "$(cat "$release_notes")" ] || die 'published release body differs from the repository notes'

  if [ "$expected_prerelease" = false ]; then
    formula="$release_tmp/sshc.rb"
    gh api -H 'Accept: application/vnd.github.raw+json' \
      "repos/$tap_repository/contents/Formula/sshc.rb" > "$formula"
    formula_tag=$(sed -n -E 's#.*archive/refs/tags/(v[0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz.*#\1#p' "$formula" | head -1)
    formula_sha=$(sed -n -E 's/^[[:space:]]*sha256 "([0-9a-f]+)"/\1/p' "$formula" | head -1)
    [ "$formula_tag" = "$tag" ] || die "Homebrew tap points to $formula_tag instead of $tag"
    source_archive="$release_tmp/source.tar.gz"
    curl -fsSL "https://github.com/$repository/archive/refs/tags/$tag.tar.gz" -o "$source_archive"
    actual_sha=$(sha256_value "$source_archive")
    [ "$formula_sha" = "$actual_sha" ] || die 'Homebrew source checksum does not match the tagged archive'
    printf 'release: Homebrew source SHA-256 OK: %s\n' "$actual_sha"
  fi

  trap - RETURN
  rm -rf -- "$release_tmp"
  printf 'release: verified https://github.com/%s/releases/tag/%s\n' "$repository" "$tag"
}

if [ "$mode" = verify ]; then
  verify_public_release
  exit 0
fi

[ -z "$(git status --porcelain)" ] || die 'worktree must be clean before publishing'
[ -f "docs/releases/$tag.md" ] || die "release notes are missing: docs/releases/$tag.md"
case "$tag" in
  *-*|*+*) ;;
  *)
    for installer_doc in README.md docs/release-install.md install.sh; do
      grep -F "SSHC_VERSION=$tag" "$installer_doc" >/dev/null || die "$installer_doc does not pin SSHC_VERSION=$tag"
      grep -F "/sshc/$tag/install.sh" "$installer_doc" >/dev/null || die "$installer_doc does not pin the installer URL to $tag"
    done
    ;;
esac

git fetch --no-tags origin '+refs/heads/main:refs/remotes/origin/main'
head_sha=$(git rev-parse HEAD)
remote_main=$(git rev-parse refs/remotes/origin/main)
[ "$head_sha" = "$remote_main" ] || die "HEAD $head_sha differs from origin/main $remote_main"
[ -z "$(git tag -l "$tag")" ] || die "local tag already exists: $tag"
[ -z "$(git ls-remote --tags origin "refs/tags/$tag")" ] || die "remote tag already exists: $tag"

ci_run=$(gh api -X GET "repos/$repository/actions/workflows/ci.yml/runs" \
  -f head_sha="$head_sha" -f branch=main -f per_page=100 |
  jq -r --arg sha "$head_sha" '
    [.workflow_runs[] |
      select(.head_sha == $sha and .head_branch == "main" and (.event == "push" or .event == "workflow_dispatch"))
    ] | sort_by(.created_at) | last | .id // empty
  ')
[ -n "$ci_run" ] || die "no main CI run exists for $head_sha; push main and wait for CI first"
printf 'release: waiting for main CI run %s\n' "$ci_run"
last_state=
while :; do
  run=$(gh api "repos/$repository/actions/runs/$ci_run")
  state=$(printf '%s' "$run" | jq -r '.status + ":" + (.conclusion // "")')
  if [ "$state" != "$last_state" ]; then
    printf 'release: CI %s\n' "$state"
    last_state=$state
  fi
  case "$state" in
    completed:success) break ;;
    completed:*) die "main CI failed: https://github.com/$repository/actions/runs/$ci_run" ;;
  esac
  sleep "$poll_seconds"
done

git fetch --no-tags origin '+refs/heads/main:refs/remotes/origin/main'
[ "$(git rev-parse refs/remotes/origin/main)" = "$head_sha" ] || die 'origin/main moved while waiting for CI'
git tag -a "$tag" "$head_sha" -m "sshc $tag"
if ! git push origin "refs/tags/$tag"; then
  git tag -d "$tag" >/dev/null
  die 'tag push failed; the newly-created local tag was removed'
fi
printf 'release: pushed annotated tag %s at %s\n' "$tag" "$head_sha"

release_run=
for _ in $(seq 1 20); do
  release_run=$(gh api -X GET "repos/$repository/actions/workflows/release.yml/runs" \
    -f head_sha="$head_sha" -f per_page=100 |
    jq -r --arg tag "$tag" --arg sha "$head_sha" '
      [.workflow_runs[] |
        select(.head_sha == $sha and .head_branch == $tag and .event == "push")
      ] | sort_by(.created_at) | last | .id // empty
    ')
  [ -z "$release_run" ] || break
  sleep 3
done
[ -n "$release_run" ] || die 'release workflow did not start after the tag push'
printf 'release: monitoring workflow https://github.com/%s/actions/runs/%s\n' "$repository" "$release_run"

last_state=
last_jobs=
while :; do
  pending=$(gh api "repos/$repository/actions/runs/$release_run/pending_deployments")
  if [ "$(printf '%s' "$pending" | jq 'length')" -gt 0 ]; then
    printf '%s' "$pending" | jq -e 'all(.[]; .environment.name == "release")' >/dev/null ||
      die 'workflow requested approval for an unexpected environment'
    while IFS= read -r environment_id; do
      [ -n "$environment_id" ] || continue
      printf 'release: approving release environment gate %s\n' "$environment_id"
      gh api --method POST "repos/$repository/actions/runs/$release_run/pending_deployments" \
        -F "environment_ids[]=$environment_id" \
        -f state=approved \
        -f comment="$tag release approved by scripts/release/publish.sh" >/dev/null
    done < <(printf '%s' "$pending" | jq -r '.[].environment.id')
  fi

  run=$(gh api "repos/$repository/actions/runs/$release_run")
  state=$(printf '%s' "$run" | jq -r '.status + ":" + (.conclusion // "")')
  jobs=$(gh run view "$release_run" --repo "$repository" --json jobs \
    --jq '[.jobs[] | (.name + "=" + .status + if .conclusion != "" then "/" + .conclusion else "" end)] | join(", ")')
  if [ "$jobs" != "$last_jobs" ]; then
    printf 'release: %s\n' "$jobs"
    last_jobs=$jobs
  fi
  if [ "$state" != "$last_state" ]; then
    printf 'release: workflow %s\n' "$state"
    last_state=$state
  fi
  case "$state" in
    completed:success) break ;;
    completed:*)
      gh run view "$release_run" --repo "$repository" --log-failed || true
      die "release workflow failed: https://github.com/$repository/actions/runs/$release_run"
      ;;
  esac
  sleep "$poll_seconds"
done

verify_public_release

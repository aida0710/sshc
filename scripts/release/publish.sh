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
  HEADとorigin/mainの一致、同じSHAのmain CIとRelease UI check（release-ui-check.yml）の
  成功を確認してannotated tagをpushし、release environmentの保護gateを承認して
  GitHub Release完了まで待ちます。

--verify-only:
  既存Releaseのchecksum、attestation、実行したバイナリのバージョン、APK、本文、Homebrew tapを検証します。
EOF
}

die() {
  printf 'release: %s\n' "$*" >&2
  exit 1
}

# release_tmp は、公開後の検証でReleaseの成果物一式を落とす一時ディレクトリである。
# 検証はdieやset -eで関数の途中から終わるので、後始末は関数の中ではなくスクリプトの
# 終了時に行い、成功しても失敗しても消す。
release_tmp=
remove_release_download() {
  [ -z "$release_tmp" ] || rm -rf -- "$release_tmp"
}
trap remove_release_download EXIT

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

# regex_literal は、tagやrepository名の`.`と`+`を、正規表現の記号ではなくその文字そのものとして照合させる。
regex_literal() {
  printf '%s' "$1" | sed 's/[.+]/\\&/g'
}

# release_signer_pattern は、attestationを署名してよいworkflowを表す証明書のSANである。
# --repoだけでは、同じrepositoryの任意のbranchやworkflowが署名したattestationも通る。
# 署名者は、このtagのpushで動いたRelease workflowか、mainから同じtagを手動復旧した
# Release workflow（workflow_dispatch）に限る。
release_signer_pattern() {
  printf '^https://github\\.com/%s/\\.github/workflows/release\\.yml@refs/(tags/%s|heads/main)$' \
    "$(regex_literal "$repository")" "$(regex_literal "$tag")"
}

verify_public_release() {
  local release expected_prerelease formula source_archive
  local expected_assets actual_assets host_asset host_os host_arch version_output
  local formula_tag formula_sha actual_sha release_notes release_body signer_pattern

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
    install.ps1 \
    "sshc-android-$tag.apk" \
    sshc-darwin-amd64 \
    sshc-darwin-arm64 \
    sshc-linux-amd64 \
    sshc-linux-arm64 \
    sshc-windows-amd64.exe \
    sshc-windows-arm64.exe | sort)
  # v0.44.0以降はVM/UI一式とそのmanifestも公開契約に含む。古いimmutable版も再検証できる。
  if [ "$(printf '%s\n' v0.44.0 "$tag" | sort -V | head -1)" = v0.44.0 ]; then
    expected_assets=$(printf '%s\n' "$expected_assets" "sshc-demo-$tag.tar.gz" "sshc-demo-$tag.json" | sort)
  fi
  actual_assets=$(printf '%s' "$release" | jq -r '.assets[].name' | sort)
  [ "$actual_assets" = "$expected_assets" ] || die 'release asset set differs from the publication contract'

  release_tmp=$(mktemp -d "${TMPDIR:-/tmp}/sshc-release-verify.XXXXXX")
  gh release download "$tag" --repo "$repository" --dir "$release_tmp"
  (
    cd "$release_tmp"
    verify_checksum_file
  )
  signer_pattern=$(release_signer_pattern)
  for artifact in "$release_tmp"/*; do
    gh attestation verify "$artifact" --repo "$repository" \
      --cert-identity-regex "$signer_pattern" --deny-self-hosted-runners >/dev/null
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

  printf 'release: verified https://github.com/%s/releases/tag/%s\n' "$repository" "$tag"
}

# vpn_snapshot_warning_days は、VPNイメージが固定したUbuntu snapshotの古さの目安である。
# 固定した時刻より後のセキュリティ修正はイメージに入らず、タグは中身から決まるので、
# Dockerfileを書き換えない限り利用者のマシンのイメージも作り直されない。月に一度は上げる。
vpn_snapshot_warning_days=30

# civil_day_number は、YYYYMMDDの日付を暦の通し日数にする。dateの日付の読み方は
# GNUとBSDで違うので、算術で数える。
civil_day_number() {
  local year=$((10#${1:0:4})) month=$((10#${1:4:2})) day=$((10#${1:6:2}))
  if [ "$month" -le 2 ]; then
    year=$((year - 1))
    month=$((month + 12))
  fi
  echo $((365 * year + year / 4 - year / 100 + year / 400 + (153 * (month - 3) + 2) / 5 + day))
}

# warn_stale_vpn_snapshot は、VPNイメージのUbuntu snapshotが古ければ警告する。時刻で
# リリースが止まらないよう、失敗にはしない。
warn_stale_vpn_snapshot() {
  local snapshot age
  snapshot=$(grep -Eo 'snapshot=[0-9]{8}T[0-9]{6}Z' internal/vpn/container/Dockerfile | head -n 1 || true)
  snapshot=${snapshot#snapshot=}
  if [ -z "$snapshot" ]; then
    printf 'release: warning: no Ubuntu snapshot time was found in internal/vpn/container/Dockerfile\n' >&2
    return 0
  fi
  age=$(($(civil_day_number "$(date -u +%Y%m%d)") - $(civil_day_number "${snapshot:0:8}")))
  if [ "$age" -gt "$vpn_snapshot_warning_days" ]; then
    printf 'release: warning: the VPN image pins Ubuntu snapshot %s (%d days old); raise it with the base digest and the InRelease hashes (docs/releasing.md)\n' \
      "$snapshot" "$age" >&2
  fi
}

if [ "$mode" = verify ]; then
  verify_public_release
  exit 0
fi

[ -z "$(git status --porcelain)" ] || die 'worktree must be clean before publishing'
warn_stale_vpn_snapshot
[ -f "docs/releases/$tag.md" ] || die "release notes are missing: docs/releases/$tag.md"
case "$tag" in
  *-*|*+*) ;;
  *) scripts/release/check-pinned-installers.sh "$tag" ;;
esac

git fetch --no-tags origin '+refs/heads/main:refs/remotes/origin/main'
head_sha=$(git rev-parse HEAD)
remote_main=$(git rev-parse refs/remotes/origin/main)
[ "$head_sha" = "$remote_main" ] || die "HEAD $head_sha differs from origin/main $remote_main"
[ -z "$(git tag -l "$tag")" ] || die "local tag already exists: $tag"
[ -z "$(git ls-remote --tags origin "refs/tags/$tag")" ] || die "remote tag already exists: $tag"

# main_workflow_run は、.github/workflows/<file>のworkflowが公開するcommitをmainで走らせたrunの
# うち、いちばん新しいもののIDを出す。無ければ何も出さない。
main_workflow_run() {
  local workflow=$1
  gh api -X GET "repos/$repository/actions/workflows/$workflow/runs" \
    -f head_sha="$head_sha" -f branch=main -f per_page=100 |
    jq -r --arg sha "$head_sha" '
      [.workflow_runs[] |
        select(.head_sha == $sha and .head_branch == "main" and (.event == "push" or .event == "workflow_dispatch"))
      ] | sort_by(.created_at) | last | .id // empty
    '
}

# wait_for_successful_run は、runが終わるまで状態の変化を「release: <label> <状態>」で表示し、
# 成功しなければfailureの文で終える。
wait_for_successful_run() {
  local run_id=$1 label=$2 failure=$3 run state last_state=
  while :; do
    run=$(gh api "repos/$repository/actions/runs/$run_id")
    state=$(printf '%s' "$run" | jq -r '.status + ":" + (.conclusion // "")')
    if [ "$state" != "$last_state" ]; then
      printf 'release: %s %s\n' "$label" "$state"
      last_state=$state
    fi
    case "$state" in
      completed:success) return 0 ;;
      completed:*) die "$failure" ;;
    esac
    sleep "$poll_seconds"
  done
}

ci_run=$(main_workflow_run ci.yml)
[ -n "$ci_run" ] || die "no main CI run exists for $head_sha; push main and wait for CI first"
# Release UI checkは手で走らせるworkflowなので、走らせ忘れをCIを待つ前に知らせる。
ui_check_run=$(main_workflow_run release-ui-check.yml)
[ -n "$ui_check_run" ] ||
  die "no Release UI check run exists for $head_sha; run 'gh workflow run release-ui-check.yml --ref main' and wait for it first (docs/releasing.md)"
printf 'release: waiting for main CI run %s\n' "$ci_run"
wait_for_successful_run "$ci_run" CI \
  "main CI failed: https://github.com/$repository/actions/runs/$ci_run; no tag was created, so fix main and run this again with $tag"
printf 'release: waiting for Release UI check run %s\n' "$ui_check_run"
wait_for_successful_run "$ui_check_run" 'Release UI check' \
  "Release UI check failed: https://github.com/$repository/actions/runs/$ui_check_run; no tag was created, so find why the embedded UI differs on that runner (docs/releasing.md) and run this again with $tag"

git fetch --no-tags origin '+refs/heads/main:refs/remotes/origin/main'
[ "$(git rev-parse refs/remotes/origin/main)" = "$head_sha" ] || die 'origin/main moved while waiting for CI and the Release UI check'
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
[ -n "$release_run" ] ||
  die "release workflow did not start after the tag push; $tag is already pushed, so find a late run or start the workflow from main as docs/releasing.md describes instead of running this again"
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
      die "release workflow failed: https://github.com/$repository/actions/runs/$release_run; if the tagged commit is sound, rebuild $tag as docs/releasing.md describes instead of making a new version"
      ;;
  esac
  sleep "$poll_seconds"
done

verify_public_release

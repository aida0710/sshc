#!/usr/bin/env bash
# CIとReleaseが同じNDK revisionを使うよう、固定値を検証して必要な場合だけ取得する。
set -euo pipefail

repository=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
version=$(tr -d '[:space:]' < "$repository/.github/android-ndk-version")
case "$version" in
  ''|*[!0-9.]*) echo "the pinned Android NDK revision is invalid" >&2; exit 1 ;;
esac

sdk_root=${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}
if [ -z "$sdk_root" ]; then
  echo "ANDROID_SDK_ROOT or ANDROID_HOME is required" >&2
  exit 1
fi
ndk_home="$sdk_root/ndk/$version"

if [ ! -d "$ndk_home" ]; then
  sdkmanager=$(command -v sdkmanager || true)
  if [ -z "$sdkmanager" ]; then
    echo "Android NDK $version is absent and sdkmanager is unavailable" >&2
    exit 1
  fi
  "$sdkmanager" "ndk;$version"
fi

properties="$ndk_home/source.properties"
if [ ! -f "$properties" ] ||
  ! grep -Eq "^Pkg.Revision[[:space:]]*=[[:space:]]*$version$" "$properties"; then
  echo "Android NDK directory does not contain revision $version" >&2
  exit 1
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  printf 'path=%s\n' "$ndk_home" >> "$GITHUB_OUTPUT"
else
  printf '%s\n' "$ndk_home"
fi
echo "Android NDK $version is ready" >&2

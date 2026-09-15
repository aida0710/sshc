#!/usr/bin/env bash
# 一時的なnetwork/runner障害だけでrelease全体をやり直さずに済むよう、
# 指定したcommandを上限付きで再実行する。
set -u

attempts=${SSHC_RETRY_ATTEMPTS:-3}
delay=${SSHC_RETRY_DELAY_SECONDS:-2}
case "$attempts" in ''|*[!0-9]*|0) echo "retry attempts must be a positive integer" >&2; exit 2 ;; esac
case "$delay" in ''|*[!0-9]*|0) echo "retry delay must be a positive integer" >&2; exit 2 ;; esac
[ "$#" -gt 0 ] || { echo "usage: retry.sh command [args...]" >&2; exit 2; }

attempt=1
while true; do
  "$@" && exit 0
  status=$?
  if [ "$attempt" -ge "$attempts" ]; then
    echo "command failed after $attempt attempts (exit $status): $1" >&2
    exit "$status"
  fi
  wait_seconds=$((delay * attempt))
  echo "command failed (attempt $attempt/$attempts); retrying in ${wait_seconds}s: $1" >&2
  sleep "$wait_seconds"
  attempt=$((attempt + 1))
done

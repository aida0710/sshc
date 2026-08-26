#!/bin/sh
set -eu

: "${ANDROID_SDK_ROOT:?set ANDROID_SDK_ROOT to an Android SDK containing platform-tools}"
SSHC_ANDROID_ADB="$ANDROID_SDK_ROOT/platform-tools/adb"
SSHC_ANDROID_APK=${1:-android/app/build/outputs/apk/debug/app-debug.apk}
SSHC_ANDROID_PACKAGE=com.github.aida0710.sshc
SSHC_ANDROID_ACTIVITY=.MainActivity
SSHC_ANDROID_ARTIFACTS=${SSHC_ANDROID_ARTIFACTS:-artifacts/android-vault-lifecycle}
SSHC_ANDROID_NODE=${SSHC_ANDROID_NODE:-node}

if [ ! -f "$SSHC_ANDROID_APK" ]; then
  echo "APK not found: $SSHC_ANDROID_APK" >&2
  exit 1
fi

mkdir -p "$SSHC_ANDROID_ARTIFACTS"
"$SSHC_ANDROID_ADB" wait-for-device
SSHC_ANDROID_API=$("$SSHC_ANDROID_ADB" shell getprop ro.build.version.sdk | tr -d '\r')
if [ "$SSHC_ANDROID_API" != "36" ]; then
  echo "Android API 36 is required, got: $SSHC_ANDROID_API" >&2
  exit 1
fi
SSHC_ANDROID_EMULATED=$("$SSHC_ANDROID_ADB" shell getprop ro.kernel.qemu | tr -d '\r')
if [ "$SSHC_ANDROID_EMULATED" != "1" ]; then
  echo "Refusing to clear sshc data on a physical Android device." >&2
  exit 1
fi

"$SSHC_ANDROID_ADB" install -r "$SSHC_ANDROID_APK" >/dev/null
"$SSHC_ANDROID_ADB" shell pm clear "$SSHC_ANDROID_PACKAGE" >/dev/null

run_phase() {
  SSHC_ANDROID_PHASE=$1
  "$SSHC_ANDROID_ADB" shell am start -W -n "$SSHC_ANDROID_PACKAGE/$SSHC_ANDROID_ACTIVITY" >/dev/null
  SSHC_ANDROID_PID=""
  SSHC_ANDROID_ATTEMPT=0
  while [ "$SSHC_ANDROID_ATTEMPT" -lt 120 ]; do
    SSHC_ANDROID_PID=$("$SSHC_ANDROID_ADB" shell pidof "$SSHC_ANDROID_PACKAGE" | tr -d '\r')
    [ -n "$SSHC_ANDROID_PID" ] && break
    SSHC_ANDROID_ATTEMPT=$((SSHC_ANDROID_ATTEMPT + 1))
    sleep 1
  done
  if [ -z "$SSHC_ANDROID_PID" ]; then
    echo "sshc process did not start" >&2
    exit 1
  fi
  "$SSHC_ANDROID_ADB" forward tcp:9222 "localabstract:webview_devtools_remote_$SSHC_ANDROID_PID" >/dev/null
  SSHC_WEBVIEW_DEBUG_ENDPOINT=http://127.0.0.1:9222 \
    "$SSHC_ANDROID_NODE" scripts/android/vault-lifecycle-test.mjs "$SSHC_ANDROID_PHASE"
  # WebView の DOM 遷移と Surface の描画完了にはわずかな時間差がある。
  sleep 1
  "$SSHC_ANDROID_ADB" exec-out screencap -p > "$SSHC_ANDROID_ARTIFACTS/$SSHC_ANDROID_PHASE.png"
}

run_phase create
"$SSHC_ANDROID_ADB" shell am force-stop "$SSHC_ANDROID_PACKAGE"
run_phase unlock
echo "Android vault lifecycle passed; screenshots: $SSHC_ANDROID_ARTIFACTS"

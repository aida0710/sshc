#!/bin/bash
# アプリの WebView の DevTools を 9223 に転送する
export PATH=~/.local/share/android-sdk/platform-tools:$PATH
pid=$(adb shell pidof com.github.aida0710.sshc.dev | tr -d '\r')
adb forward --remove tcp:9223 2>/dev/null
adb forward tcp:9223 localabstract:webview_devtools_remote_$pid >/dev/null
echo pid=$pid

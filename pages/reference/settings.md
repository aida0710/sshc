---
title: 設定
description: 外観、Terminal、input、notification、local shellの設定一覧。
---

# 設定

設定はMenu → Settingsから変更します。接続固有の項目はConnectionsのsshc tabに置かれます。

## 外観

- system／light／dark theme
- 日本語／English
- color palette、font、background、tint

Vault作成／unlock画面にもthemeと言語切替を表示します。

## Terminal

- font size
- browser scrollback lines（16 KiB〜4 MiB相当の範囲で制限）
- 選択時copy、右click paste
- OSC 52の全体既定値
- JIS円記号keyをbackslashとして送信

OSC 52と文字コードは接続ごとに上書きできます。OSC 8 linkやKitty keyboardはprotocolを検出して処理します。

描画はWebGLを優先し、利用できない環境では自動的にcanvasへfallbackします。この切替に利用者設定は不要です。

## Local shell

OS上で利用可能なshell profileを選びます。WindowsではPowerShell系、Unix系ではzsh、fish、bashなど、検出された候補から選択します。

## Notification

browser notificationは明示した操作でのみpermissionを要求します。Coding Agent連携の入力待ち／完了について、通知音、音量、test notificationを設定できます。

## 保存場所

外観などbrowserだけでよい設定はlocal storage、Terminal／接続に関わる設定はworkspaceへ保存します。Vault secretと同期credentialは平文設定fileへ保存しません。

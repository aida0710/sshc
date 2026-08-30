---
title: Android
description: Android版のinstall、local shell、navigation、file transfer。
---

# Android

![AndroidのQuick access](/images/android-home.png)

GitHub Releasesから署名済みAPKを取得します。Android 13以降では戻るgestureがsshc内の履歴へ接続され、dialog、Command Palette、navigation drawer、Inspectorを先に閉じます。

## Mobile向けの違い

- 固定bottom navigationを置かず、menu drawerで画面領域を優先
- Workspaceは分割を並べず、paneを一つずつ切り替え
- Ctrl、Alt、Esc、Tab、矢印などの特殊key row
- system file picker／Storage Access Frameworkを使うSFTP upload・download
- 外部URLはsystem browserで開く
- 転送完了／失敗をAndroid notificationで通知

## Local shell

Android app専用directory内のlocal shellを開けます。これは完全なdesktop Linux環境ではなく、Androidが提供するshellとapp sandboxの権限で動作します。`ll`や`dir`などshell alias／外部commandは標準では存在しないことがあります。

## 起動失敗時

error screenの**診断情報を表示**を開き、Version、Code、Detail、Android SDK、device、ABIを確認します。workspace pathが`/`になる、local portを確保できない、別engineと誤認する、といった起動段階の原因も詳細へ含めます。

秘密鍵、password、token、bucket secretは診断reportへ含めません。

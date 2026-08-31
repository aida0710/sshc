---
title: Android
description: Install and use the Android app, local shell, navigation, and file transfer.
---

# Android

![Quick Connect on Android](/images/android-home.png)

Install the signed APK from GitHub Releases. Android 13+ back navigation closes dialogs, Command Palette, the drawer, and Inspector before moving through page history.

## Mobile behavior

- A drawer instead of permanent bottom navigation
- One workspace pane at a time instead of a squeezed split view
- Ctrl, Alt, Esc, Tab, arrows, and other special keys
- System file picker and Storage Access Framework for SFTP
- External URLs in the system browser
- Android notifications for transfer completion and failure

Open the navigation drawer with the menu button. Edge swipe opening is disabled so it does not compete with Android's back gesture. Tap outside the drawer or use Back to close it.

The local shell runs in the app's private directory with Android sandbox permissions. It is not a full desktop Linux environment; aliases such as `ll` and tools such as `dir` may not exist.

On startup failure, open **Diagnostic details** and record Version, Code, Detail, Android SDK, device, and ABI. Reports exclude private keys, passwords, tokens, and bucket secrets.

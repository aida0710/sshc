---
title: Android
description: Install and use the Android app, local shell, navigation, and file transfer.
---

# Android

![Quick Connect on Android](/images/android-home.png)

Install the signed APK from GitHub Releases. Android 13+ back navigation closes dialogs, command search, the drawer, and Inspector before moving through page history.

## Mobile controls

- Use **Home / Connections / SFTP / Terminal / Menu** in the bottom navigation to switch screens. The navigation and header hide while the keyboard is open, leaving more room for input.
- On the terminal screen with a session open, a top bar with the session name and its destination replaces the bottom navigation and the header. The button at its left opens the drawer to move to another screen; **…** at its right also holds Find.
- Connections switches between the list and editor. Rotating a phone keeps this single-pane layout.
- Terminal scrolling continues with deceleration after a swipe. Touching again stops it; selection and leaving the screen also stop momentum. The system's reduced-motion preference is respected.
- Esc, Tab, Ctrl, and arrows occupy one row of extra keys. Open “…” for Alt and symbols. Double-tap Ctrl or Alt to lock it. The keys remain available in landscape and preserve terminal input focus.
- Tap a URL or path shown in the terminal to open a menu for opening it in the browser, copying it, and other actions. Tapping anywhere else opens the keyboard. OSC 8 hyperlinks printed by programs cannot be opened by tapping.
- Dialogs and action sheets stay within the visible area above the keyboard. Long content scrolls inside them. Android Back dismisses the topmost menu before its parent dialog.

The top-left menu button opens a drawer with running terminals and **Search everything**, which opens command search. Screen-edge gestures remain available for Android Back. Tap outside the drawer or use Back to close it.

## SFTP on a phone

The file list gets most of the screen. Its toolbar contains the host picker, Back, current folder, Search, and “…”. Tap the folder name to edit its path. Open “…” for folder creation, uploads, opening a terminal here, sorting, and other actions.

Tap a name once to open a folder or preview a file. Use a checkbox or long press to select items and reveal their actions. While selection is active, tapping another name adds or removes it from the selection. Directory navigation and remote search show progress and temporarily prevent actions on the old listing.

Transfer Manager stays in a single summary row with counts and progress. Tap it to open the detailed sheet without reducing the file list's height. **Transfer settings** opens **Settings → SFTP**. File properties also expand on demand, leaving more space for previews.

SFTP uses Android's system file picker and Storage Access Framework. Uploads accept several files at once. **Upload folder** sends a folder chosen in the Android folder picker, with its subfolders and files. Counting the selected folder as level 1, the contents of folders deeper than 64 levels are skipped. The transfer queue is limited to 200 entries, including existing transfers and files awaiting addition. Even a selection of 200 files or fewer is refused if the combined total exceeds the limit.

Transfer completion and failure can produce Android notifications. Notifications from programs in a terminal (OSC 9, 99, and 777) and long-running commands finishing also arrive as Android notifications while sshc is closed. External URLs open in the system browser.

## Development APK

The development build installs as **sshc Dev** (`com.github.aida0710.sshc.dev`) alongside the release app. Its Vault and settings use separate app storage; data from the release app is not imported automatically.

## Engine service and backups

The engine runs as a foreground service with a persistent "sshc is running" notification. Tapping **Stop** in the notification, or swiping the app away from recent tasks, ends the engine and every SSH session. Android 15 and later cap how long a data-sync service may run; the engine stops when that cap is reached. The notification permission is requested automatically once, when the first transfer finishes. To allow it later, use **Enable notifications** under **Settings → Notifications**; if Android no longer asks, allow it in Android Settings → Apps → sshc → Notifications.

App data (the vault, keys, settings, and browser registrations) is excluded entirely from Android Auto Backup and device-to-device transfer. To move to a new device, use encrypted Sync or set up the configuration and keys again there.

The sshc screen appears in screenshots and screen recordings. Take care with images that show terminal output or password fields.

## Local shell

The local shell runs in the app's private directory with Android sandbox permissions. It is not a full desktop Linux environment; aliases such as `ll` and tools such as `dir` may not exist.

## Startup failure

The failure screen lists six items: Version, Code, Detail, Android SDK, device, and ABI. **Copy diagnostic details** copies them together and **Try again** restarts the launch. The path of the folder that holds the SSH settings, or the local port result, appears inside Code and Detail when it is relevant. Reports exclude private keys, passwords, tokens, and bucket secrets.

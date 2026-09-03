---
title: Settings
description: Appearance, terminal, input, notifications, and local-shell settings.
---

# Settings

Menu is organized into feature groups. Its Settings group links directly to Engine, Terminal, Notifications, Open connections, and Master password. Preferences at the bottom of Menu controls the theme and display language. Connection-specific behavior lives in the connection's sshc tab.

## Engine

Desktop uses a device-local stable port so bookmarks and the installed web app keep the same URL. It first tries `127.0.0.1:54447`, then stores an available fallback if that port is already in use. You may change the port, but must enrol the browser again at the new origin. The native Android app manages its own local port.

## Appearance

Use Preferences for:

- System, light, or dark theme
- Japanese or English

Configure the terminal color scheme, font, background, and tint under **Settings → Terminal**.

Theme and language controls are also available on vault create and unlock screens.

## Vault auto-lock

By default, the Vault locks after saved passwords and key passphrases have not been used for 12 hours. Select a value from 1 to 999 and either minutes or hours. Status checks, terminal output, and automatic sync do not extend the timer.

With **Do not auto-lock**, the Vault stays unlocked until you lock it manually or restart sshc. Use this option only on a device you control.

## Terminal

- Maximum concurrent sessions from 1 to 200, with a default of 50
- Engine replay buffer from 16 KiB to 4 MiB, with a default of 256 KiB
- Browser scrollback from 1,000 to 100,000 lines, with a default of 5,000
- Reconnect attempts after a dropped connection and connection-log verbosity
- Font size, color scheme, font, background, and tint
- Copy on select and right-click paste
- Global OSC 52 default
- JIS yen-key to backslash behavior

The engine replay buffer retains bytes in memory so it can replay output when a browser reconnects. Browser scrollback is the separate number of display lines retained by xterm. Neither setting writes terminal output to disk.

OSC 52 and encoding may be overridden per connection. OSC 8 and Kitty keyboard behavior are protocol-driven.

Rendering uses WebGL by default and automatically falls back to the DOM renderer when WebGL is unavailable or a background image is active. If a GPU or browser duplicates characters or leaves visual traces, turn off **Use WebGL rendering** to always use DOM rendering.

## Local shell and notifications

Choose a default shell profile and start directory. Detected choices include PowerShell variants on Windows and available zsh, fish, bash, and similar shells on Unix systems.

Browser notification permission is requested only after an explicit click. Configure Coding Agent attention/completion sounds, volume, and test delivery.

The application theme, display language, notification sounds, and browser registration use browser local storage. Terminal settings, including the terminal color scheme, font, background, and tint, are stored in workspace metadata together with connection-specific settings. The browser registration token is not stored in the workspace or sync snapshots; the device keeps only its verification hash. Vault secrets and sync credentials are never written to plaintext settings.

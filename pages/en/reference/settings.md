---
title: Settings
description: Appearance, terminal, input, notifications, and local-shell settings.
---

# Settings

Open Menu → Settings. Connection-specific behavior lives in the connection's sshc tab.

## Appearance

- System, light, or dark theme
- Japanese or English
- Color palette, font, background, and tint

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

Rendering prefers WebGL and automatically falls back to canvas when unavailable; there is no user setting for this switch.

## Local shell and notifications

Choose a default shell profile and start directory. Detected choices include PowerShell variants on Windows and available zsh, fish, bash, and similar shells on Unix systems.

Browser notification permission is requested only after an explicit click. Configure Coding Agent attention/completion sounds, volume, and test delivery.

The application theme, display language, and notification sounds use browser local storage. Terminal settings, including the terminal color scheme, font, background, and tint, are stored in workspace metadata together with connection-specific settings. Vault secrets and sync credentials are never written to plaintext settings.

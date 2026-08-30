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

## Terminal

- Font size and bounded browser scrollback
- Copy on select and right-click paste
- Global OSC 52 default
- JIS yen-key to backslash behavior

OSC 52 and encoding may be overridden per connection. OSC 8 and Kitty keyboard behavior are protocol-driven.

Rendering prefers WebGL and automatically falls back to canvas when unavailable; there is no user setting for this switch.

## Local shell and notifications

Choose a detected shell profile: PowerShell variants on Windows, or available zsh, fish, bash, and similar shells on Unix systems.

Browser notification permission is requested only after an explicit click. Configure Coding Agent attention/completion sounds, volume, and test delivery.

Browser-only appearance preferences use local storage. Terminal and connection behavior is stored with the workspace. Vault secrets and sync credentials are never written to plaintext settings.

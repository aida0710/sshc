---
title: Installation
description: Install sshc on macOS, Linux, Windows or Android.
---

# Installation

sshc is a terminal app for macOS, Linux, Windows, and Android. On desktop, one `sshc` binary provides the engine, CLI, and local web UI.

## macOS / Linux

[Homebrew](https://brew.sh/) is the shortest path. Follow its official installation instructions if it is not already installed.

```sh
brew install aida0710/tap/sshc
```

Without Homebrew, pin both the installer URL and the binary version to the same release. This example installs `v0.25.0`.

```sh
SSHC_VERSION=v0.25.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.25.0/install.sh | sh'
```

After installation, `sshc update` delegates upgrades to Homebrew or to a receipt-aware installer. It shows the planned change and asks for confirmation. In non-interactive automation, review the plan and use `sshc update --yes`.

## Windows

Run this from Windows PowerShell. Administrator privileges are not required.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

It installs to `%LOCALAPPDATA%\Programs\sshc` and updates the user `PATH`. Run the same command again to update to the latest stable release.

## Android

Download `sshc-android-v<version>.apk` from [GitHub Releases](https://github.com/aida0710/sshc/releases). The release workflow verifies the signing fingerprint and checksum before publishing it.

See [Android](/en/platform/android) for mobile startup and file picker behavior.

## Start

```sh
sshc engine
```

When started in an interactive terminal, the first browser enrolment opens automatically. After service startup, if it does not open, or when adding another browser, open the UI from another terminal.

```sh
sshc
```

The engine stays in the foreground. Use tmux, systemd, launchd or another OS process manager if you want it to stay running.

Desktop first uses `http://127.0.0.1:54447/`. If another user or application already owns that port, sshc selects an available port once and stores it as this device's stable URL. Check the current URL with `sshc status`. Run `sshc` to open a one-time URL and enrol that browser. Afterwards, the same browser profile can use a bookmark or an installed web app directly, including after an engine restart on the same port. If the saved port is unavailable and the URL changes, sshc revokes the previous browser registration. Run `sshc` once against the current engine to enrol it again. Use the same command to add another browser or profile.

Chrome and Edge can install sshc from their **Install app** action. The web app does not start the engine; run `sshc engine` or keep the OS service running first.

The first launch asks you to create the vault master password.

## Keep the engine running on Ubuntu / Linux

On Linux systems using systemd, install sshc as a user service without sudo.

Stop any `sshc engine` running in the foreground or under tmux first. The systemd engine cannot start while another engine holds the lock.

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install` creates, enables, and starts `~/.config/systemd/user/sshc.service`. It reports success only after matching systemd's main PID to sshc and reaching the status API. It records Homebrew's stable `opt/sshc/bin/sshc` path or a verified `install.sh` destination. Manually copied and source-built executables are not registered automatically.

The command shows the unit and executable before asking for confirmation. Use `sshc service install --yes` when reviewed automation must skip the prompt.

sshc will not overwrite an existing hand-written unit. Stop and move that unit before switching to sshc management.

```sh
systemctl --user disable --now sshc
mv ~/.config/systemd/user/sshc.service ~/.config/systemd/user/sshc.service.manual
systemctl --user daemon-reload
sshc service install
```

To keep the user manager running after you log out, ask an administrator to enable lingering, or run this if you have permission:

```sh
loginctl enable-linger "$USER"
```

Run `sshc service disable` to stop and remove the service. It asks for confirmation and only removes a unit created by sshc. Automation can use `sshc service disable --yes`.

## Keep the engine running on macOS

On macOS, install sshc as a launchd user agent without sudo. Stop an engine running in the foreground or under tmux first.

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install` creates `~/Library/LaunchAgents/io.github.aida0710.sshc.plist`, registers it in the current GUI user domain, and starts it. It reports success only after matching launchd's PID to sshc and reaching the status API. sshc does not overwrite a hand-written plist with the same name. `sshc service disable` removes only the plist created by sshc.

## Update

- Homebrew or `install.sh`: `sshc update`
- Windows: run the PowerShell installer again
- Android: install the newer APK from GitHub Releases

When an active service was created by `sshc service install` and its executable matches the installation being updated, `sshc update` restarts it automatically. The restart locks the vault, so run `sshc vault unlock` again. If the update succeeds but only the restart fails, follow the message and run `sshc service install` again. Restart engines outside service management with `sshc engine --replace`.

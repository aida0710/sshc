---
title: Installation
description: Install sshc on macOS, Linux, Windows or Android.
---

# Installation

## macOS / Linux

Homebrew is the shortest path.

```sh
brew install aida0710/tap/sshc
```

Without Homebrew, pin both the installer URL and the binary version to the same release. This example installs `v0.21.0`.

```sh
SSHC_VERSION=v0.21.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.21.0/install.sh | sh'
```

After installation, `sshc update` delegates upgrades to Homebrew or to a receipt-aware installer.

## Windows

Run this from Windows PowerShell. Administrator privileges are not required.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

It installs to `%LOCALAPPDATA%\Programs\sshc` and updates the user `PATH`. Run the same command again to update to the latest stable release.

## Android

Download `sshc-android-v<version>.apk` from [GitHub Releases](https://github.com/aida0710/sshc/releases). The release workflow verifies the signing fingerprint and checksum before publishing it.

## Start

```sh
sshc engine
```

Open the UI from another terminal.

```sh
sshc
```

The engine stays in the foreground. Use tmux, systemd, launchd or another OS process manager if you want it to stay running.

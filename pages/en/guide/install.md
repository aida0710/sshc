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

Without Homebrew, pin both the installer URL and the binary version to the same release. This example installs `v0.46.0`.

```sh
SSHC_VERSION=v0.46.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.46.0/install.sh | sh'
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

On first launch you can set a vault master password, or continue without one.

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

launchd restarts the engine only after it fails. When the engine exits normally, for example after `sshc engine --replace`, launchd leaves it stopped. Run `sshc service install` again to return to the service.

A plist registered by an older sshc keeps the previous definition, which restarts the engine even after a normal exit, until you run `sshc service install` again. `sshc service status`, `sshc service restart`, and `sshc update` tell you to run it when this applies.

## Restart a registered service

On Linux and macOS, restart an active user service with:

```sh
sshc service restart
```

The command shows the definition and executable paths before asking for confirmation. Restarting ends existing sessions and transfers. Use `sshc service restart --yes` when automation must skip the prompt.

The service must use the stable path of this verified Homebrew or receipt-based `install.sh` installation and match the current service definition. The following cases are refused with exit code 1:

| State | Recovery |
| --- | --- |
| Absent or inactive | Run `sshc service install` to register and start it |
| Definition not managed by sshc | Inspect the hand-written definition and use its own service management procedure |
| Outdated definition | Run `sshc service install` to update the definition |
| Definition uses another executable | Run the command from the registered installation, or use `sshc service install` to switch to this installation |

A state or definition change while waiting for confirmation also prevents success. Check `sshc service status` before retrying. After restarting, the service PID must match the engine handoff and status API. A password-protected vault becomes locked, so run `sshc vault unlock`; a passwordless vault unlocks itself.

## Update

- Homebrew or `install.sh`: `sshc update`
- Windows: run the PowerShell installer again
- Android: install the newer APK from GitHub Releases

### Update from the web UI

On macOS and Linux, verified Homebrew and `install.sh` installations show an update button beside the version when a newer stable release is available. Review the current version, target version, and installation method before confirming. The installation directory must be writable. Homebrew runs `brew upgrade` and checks the version actually installed. Its formula can advance after confirmation, so a newer version may be installed. The `install.sh` installer uses the confirmed version. Manual installations, development builds, Windows, and Android show guidance for updating through their installation method.

The engine restarts after installation, ending connected sessions and transfers. Unlock a password-protected vault again after the restart. The UI shows progress and refuses duplicate updates. If installation succeeds but restarting fails, follow the restart guidance and reload the page without reinstalling.

### Update from the CLI

When an active service was created by `sshc service install` and its executable matches the installation being updated, `sshc update` restarts it automatically. The restart locks a password-protected vault, so run `sshc vault unlock` again; a passwordless vault unlocks itself when the engine starts. If the update succeeds but only the restart fails, follow the message and run `sshc service install` again. Running `install.sh` directly does not restart the service, so run `sshc service restart` afterwards if it is active. Use `sshc service install` for an inactive service or an outdated definition. Restart engines outside service management with `sshc engine --replace`.

## Uninstall

Stop the engine and remove its service registration before deleting the executable. If only the executable is deleted while the service is still registered, launchd on macOS keeps trying to start the engine every five seconds, and systemd on Linux fails to start it at every login.

1. If you registered a service with `sshc service install`, run `sshc service disable`. It stops, disables, and removes only the service definition that sshc created. Stop an engine running in the foreground or under tmux yourself.
2. Delete the executable for the way you installed it.
   - Homebrew: `brew uninstall aida0710/tap/sshc`
   - `install.sh`: delete `sshc` and the receipt `.sshc-install-receipt.json` in the same directory. The directory is normally `~/.local/bin`, `/usr/local/bin` when the script ran as root, or the directory given in `SSHC_INSTALL_DIR`.
   - `make install`: `make uninstall` deletes `~/.local/bin/sshc`.
   - Windows: stop the running engine, then run the commands below to delete `sshc.exe` and remove its directory from the user `PATH`.
   - Android: uninstall it like any other app.
3. If you used per-connection VPN, the container image `sshc-vpn` built by sshc remains. VPN containers are removed when the engine stops. To delete the image, check it with `docker image ls sshc-vpn` and remove it with `docker image rm`.

On Windows, run the following in PowerShell. If you changed the directory with `SSHC_INSTALL_DIR`, set `$dir` to that directory.

```powershell
$dir = Join-Path $env:LOCALAPPDATA 'Programs\sshc'
Remove-Item -LiteralPath (Join-Path $dir 'sshc.exe')
$entries = [Environment]::GetEnvironmentVariable('Path', 'User') -split ';' |
  Where-Object { $_ -and [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ine $dir }
[Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
```

If the executable was deleted before running `sshc service disable`, remove the service definition with these commands.

```sh
# Linux
systemctl --user disable --now sshc
rm ~/.config/systemd/user/sshc.service
systemctl --user daemon-reload

# macOS
launchctl bootout gui/$(id -u)/io.github.aida0710.sshc
rm ~/Library/LaunchAgents/io.github.aida0710.sshc.plist
```

sshc's data stays in `~/.ssh/sshc` (`%USERPROFILE%\.ssh\sshc` on Windows) after uninstalling. It holds the vault, sshc's per-connection settings, VPN profile settings, backups taken before SSH configuration changes, and deleted keys. Delete that directory only when you will not use sshc again and have confirmed you no longer need its contents; a deleted vault and its backups cannot be recovered. `~/.ssh/config`, its `Include` files, and your keys are shared with OpenSSH, so uninstalling sshc leaves them unchanged. Snapshots saved to S3-compatible storage by encrypted sync also remain; delete them on the storage side if you no longer need them.

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

Without Homebrew, pin both the installer URL and the binary version to the same release. This example installs `v0.49.0`.

```sh
SSHC_VERSION=v0.49.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.49.0/install.sh | sh'
```

After installation, `sshc update` delegates upgrades to Homebrew or to a receipt-aware installer. It shows the planned change and asks for confirmation. In non-interactive automation, review the plan and use `sshc update --yes`.

## Windows

Run this from Windows PowerShell. Administrator privileges are not required.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

It installs to `%LOCALAPPDATA%\Programs\sshc` and prepends that directory to the user `PATH`. Open a new terminal and run `sshc version`. The installer warns if another `sshc.exe` takes precedence.

### Update

Use `sshc update` or the web UI update button for installations made by `install.ps1`. Updates require an installation receipt whose SHA-256 matches the executable. Running the installation command again also updates sshc.

An engine that was running before installation restarts afterwards: through the enabled sshc task if present, or in the background otherwise. A stopped engine stays stopped. Restarting ends connected sessions and transfers and locks a password-protected vault.

### Start at logon

An installation made by the PowerShell installer can register a Task Scheduler task without administrator privileges. Stop any running engine with `sshc stop` first.

```powershell
sshc service install
sshc service status
```

`install` shows the executable path and asks for confirmation. It registers a task named `sshc Startup Task-<user SID>` in the root Task Scheduler folder and starts it immediately. The task also starts whenever this user logs on to Windows. The console window closes just after startup while the engine keeps running. Logs are appended to `%USERPROFILE%\.ssh\sshc\logs\engine.log`.

`sshc service status` reports the task state (Ready, Running, or Disabled) and whether the engine responds. Use `sshc service restart` to restart an active task. `sshc service disable` waits for the engine to shut down and deletes the task. Add `--yes` to skip confirmation for changes.

A manually created task with the same name is never overwritten or deleted. Executables run directly from Downloads and source builds cannot be registered. The installer writes `.sshc-install-receipt.json` beside the executable to verify its origin and SHA-256. Updates verify the same receipt.

## Android

Download `sshc-android-v<version>.apk` from [GitHub Releases](https://github.com/aida0710/sshc/releases). The release workflow verifies the signing fingerprint and checksum before publishing it.

See [Android](/en/platform/android) for mobile startup and file picker behavior.

## Start

```sh
sshc
```

`sshc` and `sshc open` start the engine in the background if needed, wait until it is ready, print a one-time URL and open it in your browser. The engine keeps running after you close the terminal, including on Windows. To print the URL without opening a browser, use `sshc open --no-browser`.

Stop the engine with `sshc stop`. To run it in the foreground, use `sshc engine`. Background output is appended to `~/.ssh/sshc/logs/engine.log` (`%USERPROFILE%\.ssh\sshc\logs\engine.log` on Windows). This log is excluded from encrypted sync.

Desktop first uses `http://127.0.0.1:54447/`. If another user or application already owns that port, sshc selects an available port once and stores it as this device's stable URL. Check the current URL with `sshc status`. Run `sshc` to open a one-time URL and enrol that browser. Afterwards, the same browser profile can use a bookmark or an installed web app directly, including after an engine restart on the same port. If the saved port is unavailable and the URL changes, sshc revokes the previous browser registration. Run `sshc` once against the current engine to enrol it again. Use the same command to add another browser or profile.

Chrome and Edge can install sshc from their **Install app** action. The web app does not start the engine; run `sshc` or keep the OS service running first.

On first launch you can set a vault master password, or continue without one.

## Keep the engine running on Ubuntu / Linux

On Linux systems using systemd, install sshc as a user service without sudo.

Stop any running engine with `sshc stop` first. The systemd engine cannot start while another engine holds the lock.

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

On macOS, install sshc as a launchd user agent without sudo. Stop any running engine with `sshc stop` first.

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install` creates `~/Library/LaunchAgents/io.github.aida0710.sshc.plist`, registers it in the current GUI user domain, and starts it. It reports success only after matching launchd's PID to sshc and reaching the status API. sshc does not overwrite a hand-written plist with the same name. `sshc service disable` removes only the plist created by sshc.

launchd restarts the engine only after it fails. When the engine exits normally, for example after `sshc engine --replace`, launchd leaves it stopped. Run `sshc service install` again to return to the service.

A plist registered by an older sshc keeps the previous definition, which restarts the engine even after a normal exit, until you run `sshc service install` again. `sshc service status`, `sshc service restart`, and `sshc update` tell you to run it when this applies.

## Restart a registered service

On Linux, macOS, and Windows, restart an active user service with:

```sh
sshc service restart
```

The command shows the definition and executable paths before asking for confirmation. Restarting ends existing sessions and transfers. Use `sshc service restart --yes` when automation must skip the prompt.

The service must use the stable path of this verified Homebrew, `install.sh`, or Windows `install.ps1` installation and match the current service definition. The following cases are refused with exit code 1:

| State | Recovery |
| --- | --- |
| Absent or inactive | Run `sshc service install` to register and start it |
| Definition not managed by sshc | Inspect the hand-written definition and use its own service management procedure |
| Outdated definition | Run `sshc service install` to update the definition |
| Definition uses another executable | Run the command from the registered installation, or use `sshc service install` to switch to this installation |

A state or definition change while waiting for confirmation also prevents success. Check `sshc service status` before retrying. After restarting, sshc verifies the engine handoff and status response. Linux and macOS check the service PID; Windows checks the executable running at the handoff PID. A password-protected vault becomes locked, so run `sshc vault unlock`; a passwordless vault unlocks itself.

## Update

- Homebrew, `install.sh`, or Windows `install.ps1`: `sshc update`
- Android: install the newer APK from GitHub Releases

### Update from the web UI

Open **About sshc** under **Others** in the Menu, or select the version in the sidebar, to see the running version, commit, installation method, and release notes. Notes cover the current version through the latest stable release within the 100 most recent releases retrieved. Their Markdown source is shown as plain text.

The engine checks at startup and every six hours, retaining the result. The sidebar rereads that result hourly. Select **Check for updates** on About sshc to check immediately, then **Update sshc** when available. Below the buttons, each step of a check or an update is listed with its time. If a check fails, the previously retrieved release information remains available.

Verified Homebrew and `install.sh` installations on macOS and Linux, and `install.ps1` installations on Windows, show a notice and an update button under the version when a newer stable release is available. Fold them with the mark beside the version; a folded notice stays closed until a newer stable release appears. Review the current version, target version, and installation method before confirming. The installation directory must be writable. Homebrew runs `brew upgrade` and checks the version actually installed. Its formula can advance after confirmation, so a newer version may be installed. The `install.sh` and `install.ps1` installers use the confirmed version. Manual installations, development builds, and Android show guidance for updating through their installation method.

The engine restarts after installation, ending connected sessions and transfers. Unlock a password-protected vault again after the restart. A dialog shows the progress and the result, which appears once per update. Duplicate updates are refused. If installation succeeds but restarting fails, follow the restart guidance and reload the page without reinstalling.

### Update from the CLI

On Windows, the installer stops and restarts the engine as described above. The following applies to macOS and Linux.

When an active service was created by `sshc service install` and its executable matches the installation being updated, `sshc update` restarts it automatically. The restart locks a password-protected vault, so run `sshc vault unlock` again; a passwordless vault unlocks itself when the engine starts. If the update succeeds but only the restart fails, follow the message and run `sshc service install` again. Running `install.sh` directly does not restart the service, so run `sshc service restart` afterwards if it is active. Use `sshc service install` for an inactive service or an outdated definition. Restart engines outside service management with `sshc engine --replace`.

## Uninstall

On Windows, the uninstall action below also stops the engine and removes its service registration. On macOS and Linux, stop the engine and remove its service registration before deleting the executable. If only the executable is deleted while the service is still registered, launchd on macOS keeps trying to start the engine every five seconds, and systemd on Linux fails to start it at every login.

1. If you registered a service with `sshc service install`, run `sshc service disable`. It stops, disables, and removes only the service definition that sshc created. Use `sshc stop` for an engine started outside a service.
2. Delete the executable for the way you installed it.
   - Homebrew: `brew uninstall aida0710/tap/sshc`
   - `install.sh`: delete `sshc` and the receipt `.sshc-install-receipt.json` in the same directory. The directory is normally `~/.local/bin`, `/usr/local/bin` when the script ran as root, or the directory given in `SSHC_INSTALL_DIR`.
   - `make install`: `make uninstall` deletes `~/.local/bin/sshc`.
   - Windows: use Installed apps or `uninstall.ps1` as described below; it also stops the engine and removes its task.
   - Android: uninstall it like any other app.
3. If you used per-connection VPN, the container image `sshc-vpn` built by sshc remains. VPN containers are removed when the engine stops. To delete the image, check it with `docker image ls sshc-vpn` and remove it with `docker image rm`.

On Windows, open Settings → Apps → Installed apps and uninstall sshc. Alternatively, run the following in PowerShell. If you changed the directory with `SSHC_INSTALL_DIR`, set `$dir` to that directory.

```powershell
$dir = Join-Path $env:LOCALAPPDATA 'Programs\sshc'
powershell -NoProfile -ExecutionPolicy Bypass -File "$dir\uninstall.ps1"
```

The local `uninstall.ps1` works offline. It stops the engine and removes the task created by sshc, the executable, receipt, backups, user PATH entry, and app registration. On failure it stops, shows remaining files and the steps not performed, and leaves the uninstaller available to retry after resolving the error. Data in `%USERPROFILE%\.ssh\sshc`, SSH settings, and keys in `~/.ssh` are preserved.

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

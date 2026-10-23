---
title: Run on
description: Run terminals on the sshc engine that stays up on an SSH host, while settings and SFTP stay on this computer.
---

# Run on

![Choosing where terminals run](/images/run-target-desktop.png)

**Run on** at the bottom of the sidebar chooses which sshc engine your terminals run on. Keep an sshc engine running on an SSH host (an always-on server, for example), and this computer's browser can use that engine's terminals. They keep running there after the browser or this computer is closed, which suits agents left running for a long time.

Only the terminals change: sessions and [workspaces](./workspace). Connections, SFTP, the vault and settings stay this computer's.

## Usage

1. Install the same version of sshc on the host as on this computer, and keep its engine running, for example with `sshc service install`.
2. Connect to that host once from a terminal on this computer to confirm its host key. Save any passphrase or password it needs in Connections.
3. Open **Run on** at the bottom of the sidebar and choose the host from **Add an engine…**.
4. Choose the host in the list: the workspaces and sessions become that engine's, and the terminal heading says it runs there.

Each host's **…** offers **Engine information…**, **Open its full screen in a new tab** (the host engine's own screen, like `sshc open <alias>`) and **Remove from the list**. Removing it stops neither the host's engine nor its terminals.

If the chosen host's vault is locked, sshc asks for that vault's password. This computer's engine passes it to the host's engine inside the SSH connection and does not save it here. If you cancel, unlock it at any time with **Unlock…** under Workspaces.

## Engine information

**Engine information…** shows the host engine's version, its vault state and how many sessions run on it. From there you can run the following. Each one ends every terminal on that host, so it asks first.

- **Update**: runs `sshc update --yes` on the host. Use it to bring the host up to this computer's version after updating this computer.
- **Restart**: runs `sshc service restart --yes` on the host. It works only for an engine running as the sshc-managed service.
- **Stop**: runs `sshc stop` on the host. To use it again, run `sshc open` or `systemctl --user start sshc` on the host.

After **Update** or **Restart**, a password-protected vault is locked. Unlock it with **Unlock…** in its vault row.

The dialog shows the end of what the host's `sshc` wrote.

## Requirements and limits

- sshc on the host must be the same version as on this computer. If the versions differ, the engine information dialog opens automatically. Choose **Update** to bring the host up to this computer’s version. If you close the dialog, reopen it with **Update…** under Workspaces.
- The host's SSH server must allow TCP forwarding to its `127.0.0.1` (`AllowTcpForwarding yes` or `local`).
- If the host has no vault yet, create it through **Open its full screen in a new tab**.
- Port forwarding, sending a command to several sessions, and Quick Commands work only for terminals running on this computer.
- Notifications (unread marks) are not shown for a run target that is not chosen.
- The host must run macOS or Linux.

## How it works

This computer's engine opens a single SSH connection to the host and forwards terminal operations and their input and output through it. Both machines listen on `127.0.0.1` only. It signs in to the host's engine the way that host's own `sshc` command does, without adding a browser registration there.

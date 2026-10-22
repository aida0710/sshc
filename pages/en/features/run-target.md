---
title: Run on
description: Run terminals on the sshc engine that stays up on an SSH host, while settings and SFTP stay on this computer.
---

# Run on

**Run on** at the bottom of the sidebar chooses which sshc engine your terminals run on. Keep an sshc engine running on an SSH host (an always-on server, for example), and this computer's browser can use that engine's terminals. They keep running there after the browser or this computer is closed, which suits agents left running for a long time.

Only the terminals change: sessions and [workspaces](./workspace). Connections, SFTP, the vault and settings stay this computer's.

## Usage

1. Install the same version of sshc on the host as on this computer, and keep its engine running, for example with `sshc service install`.
2. Connect to that host once from a terminal on this computer to confirm its host key. Save any passphrase or password it needs in Connections.
3. Open **Run on** at the bottom of the sidebar and choose the host from **Add an engine…**.
4. Choose the host in the list: the workspaces and sessions become that engine's, and the terminal heading says it runs there.

Each host's **…** offers **Open its full screen in a new tab** (the host engine's own screen, like `sshc open <alias>`) and **Remove from the list**. Removing it stops neither the host's engine nor its terminals.

## Requirements and limits

- sshc on the host must be the same version as on this computer. Otherwise sshc tells you to run `sshc update` there.
- The host's SSH server must allow TCP forwarding to its `127.0.0.1` (`AllowTcpForwarding yes` or `local`).
- If the host's vault is locked, unlock it through **Open its full screen in a new tab**.
- Port forwarding, sending a command to several sessions, and Quick Commands work only for terminals running on this computer.
- Notifications (unread marks) are not shown for a run target that is not chosen.
- The host must run macOS or Linux.

## How it works

This computer's engine opens a single SSH connection to the host and forwards terminal operations and their input and output through it. Both machines listen on `127.0.0.1` only. It signs in to the host's engine the way that host's own `sshc` command does, without adding a browser registration there.

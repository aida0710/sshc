---
title: Features
description: SSH and local shells, SFTP, OpenSSH connection management, reusable credentials and one-time passwords, per-connection VPN, an AI-friendly CLI, encrypted sync, and the Android app.
---

# Features

sshc is a local terminal application for SSH and local shells. It combines SFTP, port forwarding, and multiple panes with OpenSSH connection management, reusable credentials and one-time passwords, per-connection VPN, a CLI, and encrypted sync. To try it without installing, use the [public demo](https://sshc-demo.aida0710.work/index.html).

## Terminal

- Use SSH and local shells in the same interface
- Follow connection progress, and reconnect an exited SSH session while keeping its pane and scrollback
- Search, choose encodings, and open links and remote paths
- Arrange up to four panes and manage saved layouts and broadcast input in [Workspaces](./workspace)
- Use [Quick Commands and snippets](/en/terminal/commands) with confirmation before running, automatic runs on connect, and runs across several connections
- Show pane names and notifications sent by programs with [titles and notifications](/en/terminal/notifications), including notifications from Claude Code and Codex
- [Port forwarding](/en/terminal/port-forwarding) supports Local forwarding and Dynamic SOCKS, as saved settings or temporary forwards
- Change the terminal palette, font, and background in [Settings](/en/reference/settings)

## Connection management

- Parse `~/.ssh/config`, `Include` and `Match`
- Preserve comments, ordering and whitespace while editing
- Search, organize, create, duplicate, and rename connections in [Connections and groups](/en/connections/manage)
- Search hosts, configuration files, snippets and settings with `Ctrl/Cmd+K`
- Use ProxyJump, keys, and saved credentials along the same resolved route
- Run a configured `ProxyCommand` locally and use its standard input and output as the SSH transport
- Keep the same aliases available to regular ssh, VS Code, Codex, and other OpenSSH clients
- Use **Diagnostics** to explain the resolved configuration and check reachability for saved connections or a host you enter for one check
- Use **History** to review completed changes to SSH Config, keys, and other settings, recover interrupted writes, and restore individual files

## SSH keys and known hosts

- Generate, rename, move between groups, edit passphrases of, copy the public part of, and add to ssh-agent your [SSH keys](/en/connections/keys)
- Add a public key from `~/.ssh` to `authorized_keys` on one or more connections with **Remote Keys**
- Sort and review known hosts, confirm unknown host keys when connecting, and never overwrite a changed saved key automatically

## Credentials

Passwords and key passphrases are encrypted in the [vault](/en/connections/credentials) and assigned to connections or keys. Save them once, then reuse them from the terminal, SFTP, ProxyJump routes, and CLI. The vault supports automatic locking and changing the master password.

## One-time passwords (TOTP)

- Save a Base32 setup key or an `otpauth://totp/...` URI under a name in the vault
- See the current 6-digit code, and the previous and next codes when needed
- Assign a TOTP to a connection, and sshc fills in the code only when the server explicitly asks for one, such as `Verification code`. The same assignment applies to the terminal, SFTP, `sshc ssh`, and every ProxyJump hop
- Manage them from the CLI with `sshc otp list|show|add|edit|remove`

## VPN

[Per-connection VPN](./vpn) routes only the SSH connections you choose through a VPN of their own. The routes and DNS of this machine stay unchanged. It supports WireGuard, L2TP/IPsec, OpenConnect, OpenVPN, and IKEv2/IPsec, and needs Docker running on the machine. VPN secrets are kept in the vault.

## SFTP

- Browse remote files and edit text files in [SFTP](./sftp)
- Delete, rename, and create folders on the local side too; deletions ask for confirmation first
- Create symbolic links and change their targets, change owners and groups, and see free disk space on the remote side
- Change permissions of several selected entries at once, with separate modes for files and folders
- Search by name, and search the contents of several text files
- On desktop, place two remote connections side by side, compare their directories, and copy or move entries directly between them. Comparisons use size and modification time, or SHA-256 to check the contents
- Follow progress, pause, resume, and cancel in the [Transfer Manager](/en/sftp/transfers), with a speed limit shared by all transfers and automatic recovery after a connection loss
- The engine owns and persists the transfer queue so its state survives navigation and engine restarts

## CLI

- The Web UI and CLI share the same engine, OpenSSH configuration, and vault
- Codex or another AI agent can run `sshc ssh <alias> --non-interactive -- <command...>` directly; with the vault unlocked, sshc supplies the saved password or key passphrase
- Use [Serial and Telnet](/en/cli/serial-telnet) interactively or from automation
- Update with `sshc update`, register the engine as a service with `sshc service install`, and restart it with `sshc service restart`

## Encrypted sync

[Encrypted sync](./sync) encrypts connections, keys, credentials, and snippets on the device before pushing or pulling them through S3-compatible storage you provide. sshc does not provide hosted storage or retain the synced data. Plaintext is not sent to the storage provider.

## Android

The [Android app](/en/platform/android) uses the same interface for SSH, SFTP, and a local shell. On phones, a bottom navigation bar and an extra key row are available. Download the signed APK from [GitHub Releases](https://github.com/aida0710/sshc/releases).

## Updates and settings

- Installations made with `install.sh` on macOS and Linux can update to a new stable release from the Web UI. Homebrew installations use `sshc update`
- Choose the app theme (system, light, or dark), the display language (日本語 or English), keyboard shortcuts, and vault auto-lock in [Settings](/en/reference/settings)

## Public demo

The [public demo](https://sshc-demo.aida0710.work/index.html) starts three Linux VMs in your browser, so you can try SSH connections and SFTP from the Web UI and CLI without installing anything.

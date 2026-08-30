---
title: Features
description: SSH and local shells, SFTP, OpenSSH connection management, reusable credentials, an AI-friendly CLI, and encrypted sync.
---

# Features

sshc is a local terminal application for SSH and local shells. It combines SFTP, port forwarding, and multiple panes with OpenSSH connection management, reusable credentials, a CLI, and encrypted sync.

## Terminal

- Use SSH and local shells in the same interface
- Reconnect an exited SSH session while keeping its pane and scrollback
- Search, Quick Commands, encodings, Local forwarding, and Dynamic SOCKS
- Arrange up to four panes and manage saved layouts and broadcast input in [Workspaces](./workspace)

## Connection management

- Parse `~/.ssh/config`, `Include` and `Match`
- Preserve comments, ordering and whitespace while editing
- Search hosts, configuration files, snippets and settings with `Ctrl/Cmd+K`
- Use ProxyJump, keys and saved credentials along the same resolved route, while exposing ProxyCommand for analysis
- Keep the same aliases available to regular ssh, VS Code, Codex, and other OpenSSH clients

## Credentials

Passwords and key passphrases are encrypted in the vault and assigned to connections or keys. Save them once, then reuse them from the terminal, SFTP, ProxyJump routes, and CLI.

## SFTP

[SFTP](./sftp) provides remote file browsing and editing, folder transfers, pause and resume, and a background transfer queue.

## CLI

The Web UI and CLI share the same engine, OpenSSH configuration, and vault. Codex or another AI agent can run `sshc ssh <alias> --non-interactive -- <command...>` directly; with the vault unlocked, sshc supplies the saved password or key passphrase.

## Encrypted sync

[Encrypted sync](./sync) encrypts connections, keys, credentials, and snippets on the device before pushing or pulling them through S3-compatible storage you provide. sshc does not provide hosted storage or retain the synced data. Plaintext is not sent to the storage provider.

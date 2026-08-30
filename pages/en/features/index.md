---
title: Features
description: OpenSSH organization, reusable credentials, an AI-friendly CLI, and encrypted sync.
---

# Features

sshc keeps OpenSSH configuration as the source of truth, then adds reusable credentials, CLI automation, and encrypted sync around it.

## Connection management

- Parse `~/.ssh/config`, `Include` and `Match`
- Preserve comments, ordering and whitespace while editing
- Search hosts, configuration files, snippets and settings with `Ctrl/Cmd+K`
- Use ProxyJump, keys and saved credentials along the same resolved route, while exposing ProxyCommand for analysis
- Keep the same aliases available to regular ssh, VS Code, Codex, and other OpenSSH clients

## Credentials

Passwords and key passphrases are encrypted in the vault and assigned to connections or keys. Save them once, then reuse them from the terminal, SFTP, ProxyJump routes, and CLI.

## Daily workflows

- [Terminal](./terminal): search, reconnect, Quick Commands, encodings and port forwarding
- [SFTP](./sftp): folder transfers, resume, background queue and Monaco Editor
- [Workspaces](./workspace): up to four panes, drag and drop, Focus Mode and broadcast input
- [Encrypted sync](./sync): conditional push and pull with history on S3-compatible storage

## CLI

The Web UI and CLI share the same engine, OpenSSH configuration, and vault. Codex or another AI agent can run `sshc ssh <alias> --non-interactive -- <command...>` directly; with the vault unlocked, sshc supplies the saved password or key passphrase.

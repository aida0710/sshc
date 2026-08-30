---
title: Features
description: Connections, terminal, SFTP, workspaces and encrypted sync around OpenSSH.
---

# Features

sshc keeps OpenSSH configuration as the source of truth and adds a focused management surface around it.

## Connection management

- Parse `~/.ssh/config`, `Include` and `Match`
- Preserve comments, ordering and whitespace while editing
- Search hosts, configuration files, snippets and settings with `Ctrl/Cmd+K`
- Use ProxyJump, ProxyCommand, keys and saved credentials along the same resolved route

## Daily workflows

- [Terminal](./terminal): search, reconnect, Quick Commands, encodings and port forwarding
- [SFTP](./sftp): folder transfers, resume, background queue and Monaco Editor
- [Workspaces](./workspace): up to four panes, drag and drop, Focus Mode and broadcast input
- [Encrypted sync](./sync): conditional push and pull with history on S3-compatible storage

## CLI

The Web UI and CLI share the same engine and configuration resolver. Use interactive SSH, non-interactive commands, status, sync, Serial and Telnet from your shell.

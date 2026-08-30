---
title: CLI
description: The main sshc CLI commands.
---

# CLI

Use `sshc help` and each command's `--help` for the exact options supported by your installed version.

## Engine and vault

```sh
sshc engine
sshc engine --replace
sshc
sshc status --json
sshc vault create
sshc vault unlock
sshc vault lock
sshc vault change-password
```

## SSH

```sh
sshc ssh
sshc ssh --list
sshc ssh <alias>
sshc ssh <alias> --non-interactive -- <command...>
sshc info <alias> --json
```

`sshc info` resolves `Include`, `Match`, `ProxyJump` and encoding through the real connection path without an engine. It does not print saved credentials, `SetEnv` values or the `ProxyCommand` body.

## Sync

```sh
sshc sync setup
sshc sync --json
sshc sync push [--force] [--json]
sshc sync pull [--force] [--json]
sshc sync now [--json]
sshc sync auto on|off [--json]
```

## Serial / Telnet

```sh
sshc serial
sshc serial /dev/ttyUSB0 --baud 9600
sshc telnet console.example:23
```

Press `Ctrl+]` to leave an interactive Serial or Telnet connection. Telnet neither encrypts the connection nor authenticates the server.

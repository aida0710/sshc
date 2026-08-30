---
title: CLI
description: The main sshc CLI commands.
---

# CLI

Use `sshc help` and each command's `--help` for the exact options supported by your installed version.

The CLI uses the same OpenSSH configuration and the running engine's vault and sessions. For automation, use supported `--json` output instead of parsing human-readable text.

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

```sh
sshc ssh bastion --non-interactive -- uname -a
```

Non-interactive SSH fails when it would need a question, such as an unknown host key, 2FA, or an unsaved password.

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

## Terminal control

Inspect, create, and control terminals owned by the running engine.

```sh
sshc terminal list --json
sshc terminal create ssh bastion --json
sshc terminal show <session-id> --json
sshc terminal read <session-id> --cursor 0 --limit 4096 --json
sshc terminal send <session-id> --text 'uptime' --json
sshc terminal wait <session-id> --for connected --timeout 30s --json
sshc terminal rename <session-id> deploy
sshc terminal close <session-id>
```

`read` returns a scrollback cursor and warns when an older position has already been discarded. `send` checks the current process generation to avoid sending to a replacement process.

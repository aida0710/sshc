---
title: CLI
description: The main sshc CLI commands.
---

# CLI

Use `sshc help` for the full command list. Use `sshc <command...> --help` or `sshc help <command...>` for the exact arguments accepted by an individual command, for example `sshc sync push --help` or `sshc help terminal send`.

The CLI uses the same OpenSSH configuration and the running engine's vault and sessions. For automation, use supported `--json` output instead of parsing human-readable text.

Codex and other AI agents can call the CLI directly. When the vault is unlocked and the host key and credentials are already saved, non-interactive SSH uses the stored password or key passphrase. The agent does not need the credential itself.

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

The Homebrew formula installs completions for bash, zsh, and fish. For other installation methods, add the matching command below to your shell startup file. Candidates for `sshc ssh <Tab>` are read from the current `~/.ssh/config` and reachable `Include` files.

```sh
# bash
source <(sshc completion bash)

# zsh
source <(sshc completion zsh)

# fish
sshc completion fish | source
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

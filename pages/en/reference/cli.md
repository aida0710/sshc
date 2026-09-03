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
sshc service install
sshc service status
sshc service disable
sshc update
```

Interactive secrets such as the Vault master password display one `*` per typed character instead of the value. Backspace and `Ctrl+U` update the mask, and the plaintext is never written to Terminal scrollback.

`sshc service` manages a systemd user service on Linux or a launchd user agent on macOS. `install` registers a stable Homebrew or `install.sh` path, and `disable` removes only a definition created by sshc. `install`, `disable`, and `update` show the planned changes and ask for confirmation. Use `-y` or `--yes` only when automation must skip the prompt.

## SSH

```sh
sshc ssh
sshc ssh --list
sshc ssh <alias>
sshc ssh <alias> --non-interactive -- <command...>
sshc info <alias> --json
```

The Homebrew formula installs completions for bash, zsh, and fish. For other installation methods, add the matching command below to your shell startup file. Completion covers subcommands, options, enumerated values, and connection aliases for `sshc ssh`, `sshc info`, `sshc terminal create ssh`, and `sshc sftp`. Alias candidates are read from the current `~/.ssh/config` and reachable `Include` files whenever you press Tab. An alias containing characters sshc refuses to launch or evaluate (shell metacharacters, whitespace, a leading `-`) is left out of both `sshc ssh --list` and completion, and the reason is reported on stderr.

Command parsing, per-command help, and bash, zsh, and fish completion are built from the same command definition. Names and choices offered by completion therefore match `sshc help` from the installed version.

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

`sshc sync setup` shows the configured endpoint, bucket, path, region, and direction as defaults. Direction accepts `both`, `push`, or `pull`. The Access Key ID is identified only by its masked final five characters; the Secret Access Key and sync key are shown only as configured. Press Enter on blank secret prompts to keep the values already held by the engine. While a new hidden value is typed, each character appears as `*`, and Backspace updates the mask without printing the plaintext.

## SFTP transfers

Transfers use the running engine and the same OpenSSH configuration, host-key checks, and Vault credentials as the Web UI. Specify remote paths as absolute POSIX paths such as `/var/log/app.log`.

```sh
sshc sftp get bastion /var/log/app.log ./app.log
sshc sftp put bastion ./release.tar.gz /tmp/release.tar.gz
sshc sftp get bastion /srv/data ./data --recursive
sshc sftp put bastion ./public /var/www/public --recursive
sshc sftp get bastion /srv/archive ./archive --recursive --jobs 4
sshc sftp settings
sshc sftp settings --split-size 73 --split-jobs 6 --chunk-size 41
sshc sftp get bastion /backup/disk.img ./disk.img --split-size 100 --split-jobs 4 --chunk-size 512
sshc sftp put bastion ./disk.img /backup/disk.img --split-size 100 --split-jobs 4 --chunk-size 512
```

`sshc sftp settings` displays the split threshold, connections per file, and chunk size shared by Web and CLI. Add `--split-size` (16–1024 MiB), `--split-jobs` (1–8), or `--chunk-size` (8–4096 MiB) to persist only the supplied defaults. `--json` returns the saved values for automation.

`-j` or `--jobs` sets the number of files transferred concurrently from 1 to 8; the default is 1. Split options on `get` or `put` override the saved defaults for that invocation. `--split-jobs 1` disables splitting. Initial defaults are 100 MiB, four connections, and 32 MiB chunks. Combining `--jobs 4 --split-jobs 4` can use up to 16 connections, so choose values that fit the server's limits. Regular file uploads and downloads support up to 512 GiB.

In an interactive terminal, `sshc sftp get` displays one progress bar for each SFTP connection while the engine prepares the remote file. A four-connection split therefore shows four progress lines, while a non-split transfer shows one. Progress is suppressed for `--json` and non-interactive output so automation remains clean.

Existing files are never overwritten implicitly. `--overwrite` shows one confirmation before replacing them; add `--yes` only when automation must skip that confirmation. Use `--skip-existing` to preserve existing files or `--dry-run` to inspect the plan without changing anything. With `--json`, stdout contains one JSON result while progress remains on stderr. Pressing `Ctrl+C` also cancels the remote temporary upload.

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

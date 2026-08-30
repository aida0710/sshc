---
title: CLI
description: sshc CLIの主要command。
---

# CLI

正確なoptionは導入済みversionの`sshc help`と各commandの`--help`で確認してください。

## Engineとvault

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

`sshc info`はengineなしで、実接続と同じ`Include`、`Match`、`ProxyJump`、文字コードを解決します。保存済み資格情報、`SetEnv`の値、`ProxyCommand`の本文は表示しません。

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

SerialとTelnetの対話接続は`Ctrl+]`で切断します。Telnetは通信を暗号化せず、serverも認証しません。

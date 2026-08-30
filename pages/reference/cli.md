---
title: CLI
description: sshc CLIの主要command。
---

# CLI

正確なoptionは導入済みversionの`sshc help`と各commandの`--help`で確認してください。

CLIは同じOpenSSH configと、起動中engineのVault／sessionを使用します。automationではhuman-readable outputをparseせず、対応commandの`--json`を使用してください。

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

非対話commandは次の形式です。unknown host key、2FA、未保存passwordなど、質問が必要な状態では失敗します。

```sh
sshc ssh bastion --non-interactive -- uname -a
```

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

## Terminal操作

起動中engineのTerminalを別processから調査、作成、操作できます。

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

`read`は保持済みscrollbackのcursorを返します。古い位置が既に破棄されている場合は、保持中の先頭へ進めたことを警告します。`send`は対象process generationを検証し、session IDの再利用による誤送信を避けます。

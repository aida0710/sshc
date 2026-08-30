---
title: CLI
description: sshc CLIの主なコマンドと、自動化で使うときの注意点。
---

# CLI

利用できるオプションはバージョンによって異なります。正確な内容は、インストール済みの`sshc help`と各コマンドの`--help`で確認してください。

CLIはWeb UIと同じOpenSSH設定を読み、起動中のエンジンが管理するVaultやセッションを利用します。自動化では、画面表示用の文を解析せず、対応するコマンドの`--json`を使用してください。

CodexなどのAIエージェントからも直接実行できます。Vaultのロックが解除され、接続先のホスト鍵と認証情報が保存されていれば、非対話SSHではsshcがパスワードや鍵のパスフレーズを認証に使います。AIエージェントへ認証情報そのものを渡す必要はありません。

## エンジンとVault

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

`sshc info`はエンジンを起動せずに、実際の接続と同じ`Include`、`Match`、`ProxyJump`、文字コードを解決します。保存済みの資格情報、`SetEnv`の値、`ProxyCommand`の本文は表示しません。

非対話コマンドは次の形式です。未知のホスト鍵、2FA、未保存のパスワードなど、利用者への確認が必要な場合は失敗します。

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

SerialとTelnetの対話接続は`Ctrl+]`で切断します。Telnetは通信を暗号化せず、サーバーも認証しません。

## Terminal操作

起動中のエンジンが管理するTerminalを、別のプロセスから確認、作成、操作できます。

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

`read`は、保持しているスクロールバックのカーソル位置を返します。古い位置がすでに破棄されている場合は、現在保持している先頭まで進めたことを警告します。`send`は対象プロセスの世代を検証し、セッションIDの再利用による誤送信を防ぎます。

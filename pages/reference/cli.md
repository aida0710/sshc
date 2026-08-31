---
title: CLI
description: sshc CLIの主なコマンドと、自動化で使うときの注意点。
---

# CLI

利用できるオプションはバージョンによって異なります。インストール済みの`sshc help`で全体の一覧を、`sshc <command...> --help`または`sshc help <command...>`で各コマンドの正確な引数を確認できます。例えば`sshc sync push --help`と`sshc help terminal send`を利用できます。

CLIはWeb UIと同じOpenSSH設定を読み、起動中のエンジンが管理しているVaultやセッションを利用しています。自動化では、人向けの表示文を解析せず、対応するコマンドの`--json`を利用できます。

CodexなどのAIエージェントからも直接実行できます。Vaultのロックが解除され、接続先のホスト鍵と認証情報が保存されていれば、非対話SSHの認証にはsshcがパスワードや鍵のパスフレーズを使っています。AIエージェントへ認証情報そのものを渡す必要はありません。

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
sshc service install
sshc service status
sshc service disable
```

`sshc service`はLinuxのsystemdユーザーサービスを管理します。`install`はHomebrewまたは`install.sh`で導入された安定パスを登録し、`disable`はsshcが作成したunitだけを削除します。

## SSH

```sh
sshc ssh
sshc ssh --list
sshc ssh <alias>
sshc ssh <alias> --non-interactive -- <command...>
sshc info <alias> --json
```

`sshc info`では、エンジンを起動せずに、実際の接続と同じ`Include`、`Match`、`ProxyJump`、文字コードの解決結果を確認できます。保存済みの認証情報、`SetEnv`の値、`ProxyCommand`の本文は表示されません。

非対話コマンドは次の形式です。未知のホスト鍵、2FA、未保存のパスワードなど、利用者への確認が必要な状態では実行できません。

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

SerialとTelnetの対話接続は`Ctrl+]`で切断できます。Telnetは通信を暗号化せず、サーバーも認証しません。

## Terminal操作

起動中のエンジンが管理するTerminalセッションを別のプロセスから確認、作成、操作できます。

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

`read`では、保持しているスクロールバックと次の読み取り位置を取得できます。指定した位置がすでに破棄されている場合は、読み取り位置が現在残っている先頭まで進められたことが警告として返ります。`send`では対象プロセスの世代を検証し、セッションIDの再利用による誤送信を防いでいます。

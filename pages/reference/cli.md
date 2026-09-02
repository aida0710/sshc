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
sshc update
```

`sshc service`はLinuxではsystemdユーザーサービス、macOSではlaunchdユーザーエージェントを管理します。`install`はHomebrewまたは`install.sh`で導入された安定パスを登録し、`disable`はsshcが作成した定義だけを削除します。`install`、`disable`、`update`は変更内容を表示してから確認を求めます。自動化で確認を省略する場合だけ`-y`または`--yes`を付けてください。

## SSH

```sh
sshc ssh
sshc ssh --list
sshc ssh <alias>
sshc ssh <alias> --non-interactive -- <command...>
sshc info <alias> --json
```

Homebrew版ではbash、zsh、fishの補完が一緒に導入されます。その他の導入方法では、利用中のシェルに合わせて次のいずれかを起動設定へ追加してください。サブコマンド、オプション、列挙値に加え、`sshc ssh`、`sshc info`、`sshc terminal create ssh`、`sshc sftp`では接続先も補完します。接続先候補は、Tabを押した時点の`~/.ssh/config`と到達可能な`Include`から取得されます。`sshc`が起動も評価もしないと定めた文字（shellメタ文字、空白、先頭の`-`など）を含むaliasは`sshc ssh --list`にも補完にも出さず、除外した理由をstderrに表示します。

コマンドの解釈、個別ヘルプ、bash／zsh／fishの補完は、同じコマンド定義から作られています。補完に表示される名前や選択肢は、そのバージョンの`sshc help`と一致します。

```sh
# bash
source <(sshc completion bash)

# zsh
source <(sshc completion zsh)

# fish
sshc completion fish | source
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

`sshc sync setup`は、設定済みのEndpoint、Bucket、Path、Region、Directionを既定値として表示します。Directionは`both`、`push`、`pull`から選びます。Access Key IDは末尾5文字だけを伏せ字付きで表示し、Secret Access Keyと同期鍵は値を表示せず「設定済み」と示します。再設定時は秘密値を空のままEnterすると、engine内の既存値を維持します。新しい値の入力中は、平文の代わりに1文字ずつ`*`を表示し、Backspaceも画面へ反映します。

## SFTP転送

起動中のエンジンと、Web UIと同じOpenSSH設定、Host Key検証、Vaultの認証情報を使って転送します。リモートパスは`/var/log/app.log`のような絶対POSIXパスで指定します。

```sh
sshc sftp get bastion /var/log/app.log ./app.log
sshc sftp put bastion ./release.tar.gz /tmp/release.tar.gz
sshc sftp get bastion /srv/data ./data --recursive
sshc sftp put bastion ./public /var/www/public --recursive
```

既存ファイルは暗黙に上書きしません。上書きする場合は`--overwrite`を付けると実行前にまとめて確認し、`--yes`を併用した場合だけ確認を省略します。既存ファイルを残す場合は`--skip-existing`、変更せず転送計画だけ確認する場合は`--dry-run`を利用できます。自動化では`--json`を付けるとstdoutへ1つのJSON結果を出し、進捗はstderrへ分離します。`Ctrl+C`で中断したアップロードは、リモートの一時ファイルも取り消します。

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

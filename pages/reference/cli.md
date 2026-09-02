---
title: CLI
description: sshc CLIの主なコマンドと、自動化で使うときの注意点。
---

# CLI

利用できるオプションはバージョンによって異なります。インストール済みの`sshc help`で全体の一覧を、`sshc <command...> --help`または`sshc help <command...>`で各コマンドの正確な引数を確認できます。例えば`sshc sync push --help`と`sshc help terminal send`を利用できます。

SSH接続、SFTP転送、同期、Terminal操作の各コマンドは、起動中のエンジンと、そのエンジンが管理するOpenSSH設定、Vault、セッションを使います。`sshc info`や`sshc completion`など、情報をローカルで読み取るだけのコマンドはエンジンを必要としません。自動化では、人向けの表示文を解析せず、対応するコマンドの`--json`を利用できます。

CodexなどのAIエージェントからも直接実行できます。非対話SSHでは、Vaultのロックが解除され、接続経路にあるすべてのホスト鍵が登録済みで、パスワードや鍵のパスフレーズを保存済みである必要があります。2FA、未登録のホスト鍵、未保存の認証情報など、利用者の入力や判断が必要な接続は実行できません。条件を満たす場合はsshcが認証するため、AIエージェントへ認証情報そのものを渡す必要はありません。

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

Vaultのマスターパスワードなどを対話入力すると、入力した値の代わりに`*`を表示します。入力値がTerminalのスクロールバックへ平文で残ることはありません。

`sshc service`はLinuxではsystemdユーザーサービス、macOSではlaunchdユーザーエージェントを管理します。`install`はHomebrewまたは`install.sh`で導入された安定パスを登録し、`disable`はsshcが作成した定義だけを削除します。`install`、`disable`、`update`は変更内容を表示してから確認を求めます。自動化で確認を省略する場合だけ`-y`または`--yes`を付けてください。

## SSH

```sh
sshc ssh
sshc ssh --list
sshc ssh <alias>
sshc ssh <alias> --non-interactive -- <command...>
sshc info <alias> --json
```

Homebrew版ではbash、zsh、fishの補完が一緒に導入されます。その他の導入方法では、利用中のシェルに合わせて次のいずれかをシェルの初期化ファイルへ追加してください。サブコマンド、オプション、列挙値に加え、`sshc ssh`、`sshc info`、`sshc terminal create ssh`、`sshc sftp`では接続先も補完します。接続先候補は、Tabを押した時点の`~/.ssh/config`と到達可能な`Include`から取得されます。シェル展開やコマンド連結につながるメタ文字、空白、先頭の`-`などを含むエイリアスは、`sshc ssh --list`と補完候補から除外し、理由を標準エラー出力へ表示します。

コマンドの解釈、個別ヘルプ、bash／zsh／fishの補完は、同じコマンド定義から作られています。補完に表示される名前や選択肢は、そのバージョンの`sshc help`と一致します。

```sh
# bash
source <(sshc completion bash)

# zsh
source <(sshc completion zsh)

# fish
sshc completion fish | source
```

`sshc info`では、エンジンを起動せずに、実際の接続時と同じ規則で`Include`、`Match`、`ProxyJump`、文字コードを読み、最終的に使われる値を確認できます。保存済みの認証情報、`SetEnv`の値、`ProxyCommand`の本文は表示されません。

非対話コマンドは次の形式です。

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

`sshc sync setup`は、設定済みのエンドポイント、バケット、パス、リージョン、同期方向を既定値として表示します。同期方向は`both`、`push`、`pull`から選びます。Access Key IDは末尾5文字だけを伏せ字付きで表示し、Secret Access Keyと同期キーは値を表示せず「設定済み」と示します。再設定時は秘密値を空のままEnterキーで進むと、エンジンに保存済みの値を維持します。新しい値の入力中は、平文の代わりに`*`を表示します。

## SFTP転送

起動中のエンジンと、Web UIと同じOpenSSH設定、ホスト鍵の検証、Vaultの認証情報を使って転送します。リモートパスは`/var/log/app.log`のような絶対POSIXパスで指定します。

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

`sshc sftp settings`は、WebとCLIが共通で使う分割開始サイズ、1ファイルの接続数、チャンクサイズを表示します。同じコマンドへ`--split-size`（16〜1024 MiB）、`--split-jobs`（1〜8）、`--chunk-size`（8〜4096 MiB）を付けると、指定した項目だけを既定値として保存します。`--json`では保存後の値を機械可読形式で取得できます。

`-j`または`--jobs`には、同時に転送するファイル数を1〜8で指定します。既定は1です。`get`／`put`へ分割オプションを付けた場合は、保存済みの既定値をその実行だけ上書きします。`--split-jobs 1`は分割しません。初期値は100 MiB以上、4接続、32 MiBチャンクです。複数ファイルを`--jobs 4 --split-jobs 4`で転送すると最大16接続になり得るため、接続先の上限に合わせて指定してください。通常ファイルはアップロード、ダウンロードともに512 GiBまで転送できます。

既存ファイルは暗黙に上書きしません。上書きする場合は`--overwrite`を付けると実行前にまとめて確認し、`--yes`を併用した場合だけ確認を省略します。既存ファイルを残す場合は`--skip-existing`、変更せず転送計画だけ確認する場合は`--dry-run`を利用できます。自動化では`--json`を付けると標準出力へ1つのJSON結果を出し、進捗は標準エラー出力へ分離します。`Ctrl+C`で中断したアップロードでは、sshcがリモート側に作成した一時ファイルも削除します。

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

`read`では、保持しているスクロールバックと次回指定する読み取り位置を取得できます。指定した位置の出力がすでに破棄されている場合は、現在残っている先頭から返し、そのことを警告に含めます。`send`は、確認後にセッション内のプロセスが入れ替わっていた場合には何も送信しません。

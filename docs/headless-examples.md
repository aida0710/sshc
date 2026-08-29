# エンジンを常駐させる

`sshc engine` はフォアグラウンドで動作し、起動したターミナルまたはプロセス管理ツールが終了すると停止します。sshc 自身はデーモン化しません。

マスターパスワードを unit ファイル、環境変数、コマンドライン引数へ保存しないでください。エンジンの起動後、別のターミナルから vault のロックを解除します。

```sh
sshc engine                       # Ctrl+C で終了コード 130、SIGTERM で 0
sshc vault unlock                 # 別のターミナルから実行
sshc ssh <接続先>                 # 保存済みの資格情報を利用して接続
sshc ssh <接続先> --non-interactive -- <コマンド...>
                                  # ターミナルを開かずにコマンドを実行
```

## 画面なし環境で同期する

同期の owner は常駐中の engine です。先に engine を起動し、別の対話端末で vault を解錠します。`sshc sync` を含むすべての同期コマンドは、engine 停止中、vault 未作成、vault ロック中をそれぞれ拒否して復旧コマンドを表示します。

```sh
sshc vault unlock
sshc sync setup
sshc sync
sshc sync push
sshc sync pull
sshc sync now
sshc sync auto on
```

`sshc sync setup` は stdin と prompt 出力の両方に対話端末を要求します。object storage の access key ID、secret access key、既存の sync key は no-echo で読み取り、argv、環境変数、設定ファイルから渡す option はありません。空の同期先で生成された sync key は同じ端末へ一度だけ表示されるため、別端末の復元に備えて安全に保管してください。systemd の unit や `tmux send-keys` に資格情報を書かないでください。

通常の `push` は engine が作った draft を条件付きで保存し、remote が変われば失敗します。`push --force` も確認時の exact remote ETag だけを対象とし、競合時に自動再試行しません。通常の `pull` は conflict または removal が一件でもあれば preview だけで止まり、`pull --force` はその exact preview を remote authoritative として適用します。preview 後に ETag／revision が変われば force でも拒否されます。

自動処理で結果を読む場合は、対応する操作へ `--json` を付けます。stdout には一つの JSON object だけが出ます。`setup` は秘密入力を伴うため JSON mode を持ちません。

```sh
sshc sync --json
sshc sync push --json
sshc sync pull --force --json
sshc sync now --json
sshc sync auto off --json
```

接続設定だけを調べる `sshc info <alias> [--json]` は engine を必要としません。実接続と同じ `Include`／`Match`／`ProxyJump` 解決を使いますが、保存済み資格情報、`SetEnv` の値、`ProxyCommand` の本文は表示しません。

## systemd（ユーザーサービス）

`~/.config/systemd/user/sshc.service`:

次の例はインストールスクリプトの既定パスを使用します。Homebrew などで別の場所へインストールした場合は、`command -v sshc` の結果に合わせて `ExecStart` を変更してください。

```ini
[Unit]
Description=sshc engine
After=default.target

[Service]
Type=simple
ExecStart=%h/.local/bin/sshc engine
Restart=on-failure
SuccessExitStatus=130

[Install]
WantedBy=default.target
```

`Type=simple` は、エンジンがフォアグラウンドで動作することを表します。`SuccessExitStatus=130` により、Ctrl+C で停止した場合の自動再起動を防ぎます。

```sh
systemctl --user daemon-reload
systemctl --user enable --now sshc
sshc vault unlock
```

sshc は unit ファイルを自動作成しません。配置場所と起動方法は利用者が管理します。

## tmux

```sh
tmux new-session -d -s sshc 'sshc engine'
tmux new-window -t sshc 'sshc vault unlock; exec $SHELL'
```

`sshc vault unlock` は対話端末を必要とします。`tmux send-keys` でパスワードを送信すると履歴やログに残る可能性があるため、使用しないでください。

## Docker

```dockerfile
FROM debian:stable-slim
COPY sshc /usr/local/bin/sshc
RUN install -d -o 1000 -g 1000 /home/sshc
USER 1000:1000
ENV HOME=/home/sshc
ENTRYPOINT ["sshc", "engine"]
```

```sh
docker run -d --name sshc -v sshc-home:/home/sshc sshc
docker exec -it sshc sshc vault unlock
docker exec -it sshc sshc ssh <接続先>
```

vault のロック解除には対話端末が必要なので、`docker exec` に `-it` を指定します。マスターパスワードを `docker run -e` で渡す機能はありません。

エンジンはコンテナ内のループバックアドレスで待ち受けます。ホスト側の `sshc` からこのエンジンへ接続することはできないため、UI を使わない運用では接続コマンドも `docker exec` で実行します。

## Windows（PowerShell）

```powershell
sshc engine
```

別の PowerShell から vault のロックを解除します。

```powershell
sshc vault unlock
```

タスクスケジューラへ登録する場合は、対話的なログオン時に通常のユーザー権限で実行してください。エンジンは実行ユーザーの `~/.ssh` を使用するため、別のユーザーや SYSTEM として起動すると異なる SSH 設定を参照します。

sshc は Windows サービスをインストールしません。Windows サービスには対話端末がなく、vault をターミナルから解除する運用と一致しないためです。

## 終了コード

| 終了条件 | 終了コード | 意味 |
| --- | --- | --- |
| Ctrl+C / SIGINT | 130 | 利用者が停止した |
| SIGTERM | 0 | プロセス管理ツールが停止した |
| エンジンロックの取得失敗 | 1 | 同じユーザーのエンジンが既に動作している |

Ctrl+C の終了コード 130 を正常終了として扱うよう、プロセス管理ツールを設定してください。systemd では上の `SuccessExitStatus=130` が該当します。

SIGINT と SIGTERM の終了動作は `integration/signals_unix_test.go`、Windows の Ctrl+Break は `integration/signals_windows_test.go` で実プロセスに対して検証しています。

## vault の自動ロック

常駐モードでは、vault は 12 時間操作がない場合に自動的にロックされます。ロック後もエンジンは動作を続けますが、保存済みのパスワードや鍵パスフレーズを必要とする操作は、`sshc vault unlock` を実行するまで利用できません。

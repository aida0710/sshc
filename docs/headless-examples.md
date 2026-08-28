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

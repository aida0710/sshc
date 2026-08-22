# 端末で engine を持つ

`sshc headless` は、この端末が engine の持ち主になる、という意味である。画面の
無い機械で使う入口であり、**前面で走り続ける。** 止まれば engine も止まる。

**マスターパスワードは、どの例にも書かない。** unit ファイルにも、環境変数にも、
コマンドラインにも置かない。engine を起こしたあと、**別の端末から**
`sshc vault unlock` を打つ。それが端末を要求するのは、書ける場所へ書かせない
ためである。

```sh
sshc headless          # この端末が持つ。Ctrl-C で 130、SIGTERM で 0
sshc vault unlock      # 別の端末から。同じ engine を開ける
sshc <接続先>           # 解錠済みの engine に材料を求めて繋ぐ
sshc run <接続先> <コマンド...>   # 端末を開かずに一つ走らせる
```

## systemd（利用者の unit）

```ini
# ~/.config/systemd/user/sshc.service
[Unit]
Description=sshc engine
After=default.target

[Service]
# **前面のまま置く。** これは自分で背後へ回らないので、Type=simple が正しい。
Type=simple
ExecStart=%h/.local/bin/sshc headless
Restart=on-failure
# Ctrl-C の 130 は「人が止めた」という意味であり、失敗ではない。
# これを付けないと、手で止めるたびに systemd が起こし直す。
SuccessExitStatus=130

[Install]
WantedBy=default.target
```

```sh
systemctl --user daemon-reload
systemctl --user enable --now sshc
# 解錠は別の端末から。unit には何も書かない。
sshc vault unlock
```

**sshc はこの unit を書きません。** 以前は自分で書いていたが、その仕組みごと
削除した。置くかどうかも、置く場所も、利用者が決めることである。

## tmux

```sh
tmux new-session -d -s sshc 'sshc headless'
tmux new-window -t sshc 'sshc vault unlock; exec $SHELL'
```

解錠のために窓を分けるのは、`sshc vault unlock` が端末を要求するからである。
`tmux send-keys` でパスワードを送ってはならない — **それは打鍵ではなく、
履歴とログに残る文字列である。**

## Docker

```dockerfile
FROM debian:stable-slim
COPY sshc /usr/local/bin/sshc
USER 1000:1000
ENV HOME=/home/sshc
ENTRYPOINT ["sshc", "headless"]
```

```sh
docker run -d --name sshc -v sshc-home:/home/sshc sshc
# 解錠は、この容器の中の端末から。
docker exec -it sshc sshc vault unlock
```

`-it` が要るのは、ここでも端末が要求されるからである。`docker run -e` で
パスワードを渡す道は無い。**無いのは、作らなかったからである。**

## Windows（PowerShell）

```powershell
# この端末が持つ。閉じれば engine も終わる。
sshc headless
```

別の PowerShell から:

```powershell
sshc vault unlock
```

**タスク スケジューラに登録するなら、対話的なログオン時に、最上位の権限を
要求せずに。** engine は利用者の `~/.ssh` を開くものなので、別の利用者や
SYSTEM として走らせると、開く家が変わる。

**sshc は Windows のサービスを入れません。** サービスは利用者のいない文脈で
走るものであり、端末から解錠するという前提と噛み合わない。

## 監督者へ渡す約束

| 終わり方 | 終了コード | 意味 |
| --- | --- | --- |
| Ctrl-C / SIGINT | 130 | 人が止めた |
| SIGTERM | 0 | 監督者が止めた |
| 所有権チャンネルの EOF（`sshc engine`） | 0 | 外殻が手を離した |
| engine lock を取れなかった | 1 | 既に誰かが持っている |

**130 を失敗として扱う監督者は多い。** 手で止めるたびに起こし直されないよう、
`SuccessExitStatus=130` に相当する設定を入れておくこと。

これらは `integration/signals_unix_test.go` と `signals_windows_test.go` が
実プロセスに対して確かめている。

## 保管庫は 12 時間で閉じます

**ここだけ、時計で閉じます。** デスクトップの engine はアプリの子で、アプリを
終えれば一緒に死にます——蓋を閉じたノートも、開けば OS がログインパスワードを
訊きます。**`sshc headless` にはそのどちらもありません。** systemd の下で何週間も
走り続ける engine にとって、12 時間の自動施錠が唯一の歯止めです。

閉じたあと、保管庫を要る操作は `sshc vault unlock` を待ちます。

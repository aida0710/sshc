# 安定インストールとログインサービス再バインド

## 背景

`make install` は現在、production build を `~/.local/bin/sshc` へコピーする。しかし、既に有効なログイン時起動は、それを有効にしたプロセス自身の絶対パスを保持する。実機ではコマンド検索が `~/.local/bin/sshc` を返す一方、LaunchAgent はリポジトリ内の `bin/sshc` を起動しており、両ファイルの内容も異なっていた。

この状態では「インストール済みのコマンド」「ログイン時から動くサーバー」「OpenSSH が `SSH_ASKPASS` として呼ぶ実行ファイル」が同じリリースである保証がない。リポジトリを移動または削除すると、ログイン時起動も壊れる。

ここでいう正式なインストールはシステム全体への配置ではない。sshc の vault、設定、ログインサービスはすべてユーザー単位なので、sudo の必要な `/usr/local/bin` より、既存の `~/.local/bin/sshc` を安定したユーザー領域の配置先とする方が境界に合っている。

## 目的

- `make install` 後、CLI と有効なログインサービスが同じ `$(INSTALL_DIR)/sshc` を使う。
- 更新中の実行ファイルを直接上書きせず、ステージしたファイルを rename して置き換える。
- ログイン時起動が有効な場合だけ、新しいパスへ再登録して再起動する。
- ログイン時起動が無効なら、インストールを理由に勝手に有効化しない。
- `make uninstall` はログインサービスを停止・削除した後でバイナリを削除する。
- macOS と Linux で既存の LoginItem 実装を再利用し、Makefile に OS 固有設定形式を複製しない。

## 非目的

- root 権限を使うシステム全体インストール
- Homebrew、deb、rpm などのパッケージ作成
- コード署名、公証、リリース配布形式の変更
- インストール時にログイン時起動を新規に有効化すること
- vault の解錠状態をプロセス再起動後も維持すること

## 検討した案

### A. sshc 自身がログインサービスを更新する（採用）

保守用の `sshc service refresh` と `sshc service disable` を追加する。`make install` は新しく配置したバイナリ自身に refresh を依頼し、`make uninstall` はリポジトリでビルドした同じ版に disable を依頼する。

利点は、plist・systemd unit・launchctl・systemctlの知識が既存のplatform実装に一か所だけ残ること、任意の実行パスを引数として受け取らず `os.Executable()` の絶対パスだけを登録できることである。欠点は `service` がCLIの予約語になり、その名前のSSH aliasはsshc経由では使えなくなることである。

### B. Makefile がOSを判定して直接更新する

実装量は少なく見えるが、plist XML、systemd unit、標準出力へbootstrap tokenを残さない規則、再起動手順を二重管理することになる。Web設定と `make install` が時間とともに異なるサービスを作るため採用しない。

### C. コピーだけ行い、手動で設定を切り替えてもらう

現在の挙動に警告を足すだけで安全だが、「インストールしたのに常駐版は古い」という問題を解決しない。利用者が一人でも、更新のたびに手順を覚えて実行する設計は導線として弱いため採用しない。

## CLI契約

### `sshc service refresh`

1. ユーザーのログインサービスが未登録なら何も変更せず成功する。
2. 登録済みなら、現在実行中のsshc自身を `os.Executable()` で絶対パスとして解決する。
3. 既存の LoginItem `Enable` を同じパスで再実行し、設定ファイルを書き直してサービスを再起動する。
4. 再起動後のサーバーは通常どおり施錠状態から始まる。開いていたWebセッションは切れ、`sshc open` または通常の `sshc` から入り直す。
5. refresh はログインサービスを新規には有効化しない。

保守コマンドはWeb画面用のplatform部品が公開されているかどうかには依存せず、各OSのLoginItemを直接組み立てる。特にLinuxでは、unitファイルが存在する一方で`systemctl`を実行できない場合を「未登録」とは扱わず、説明付きのエラーにする。登録もunitも存在しない環境だけが安全なno-opである。

### `sshc service disable`

1. 未登録なら何も変更せず成功する。
2. 登録済みなら既存の LoginItem `Disable` を使って停止・設定削除を行う。
3. 停止または削除に失敗した場合は非zeroで終了し、uninstallがバイナリだけを消して壊れたサービスを残さないようにする。

どちらのコマンドもWebサーバー、ブラウザ、SSH接続を開始しない。標準出力へbootstrap URLや秘密を出さず、結果だけを一行で報告する。

## Makefile契約

### `make install`

1. 通常どおりproduction UIとGoバイナリをビルドする。
2. `$(INSTALL_DIR)` を作る。
3. 同じディレクトリ内の一時ファイルへ mode `0755` でコピーする。
4. rename で `$(INSTALL_DIR)/sshc` と置き換える。実行中の古いinodeは、再起動までそのプロセスだけが保持する。
5. 新しく配置した `$(INSTALL_DIR)/sshc service refresh` を実行する。
6. PATHに含まれない場合の既存の注意を維持する。

ログインサービスの更新に失敗した場合、バイナリの配置成功を取り消さないが、makeは失敗として終了し、「CLIは更新されたがサービスは更新できなかった」と分かる状態にする。再実行可能であることを優先し、失敗を成功として隠さない。

### `make update`

既存の `git pull --ff-only` と `make install` を維持する。したがって更新後は、有効なログインサービスも自動的に同じ版へ移る。

### `make uninstall`

1. 現在のソースから保守サブコマンドを含むバイナリをビルドする。
2. そのバイナリで `service disable` を実行する。
3. disableに成功した場合だけ `$(INSTALL_DIR)/sshc` を削除する。

uninstallのために一度ビルドするのは、インストール済みバイナリがこの機能を持たない旧版でも、`service`をSSH aliasと誤解して接続を始めず、安全にサービスを解除できるようにするためである。

## OS別の再起動

- **macOS:** 既存の plist更新 → `bootout` → `bootstrap` を使う。これによりリポジトリ内の古いプロセスは止まり、安定パスの新しいプロセスが始まる。
- **Linux:** unit更新 → `daemon-reload` → `enable` → `restart` とする。既存の `enable --now` だけでは、既に動いているサービスの `ExecStart` が切り替わらない可能性があるため、明示的にrestartする。

UIでログイン時起動を切り替える既存APIも同じ LoginItem 実装を使うため、Linux側の再起動規則はUIからの再登録にも一貫して適用される。

## エラーと安全境界

- 登録するパスは任意のCLI引数ではなく、実行中バイナリの絶対パスだけにする。
- 相対パス、改行、systemdの `%`、plistのXML特殊文字に対する既存防護を維持する。
- service refresh中の失敗は、vaultやSSH設定ファイルを変更しない。
- refreshは登録済み状態を検査してから動き、オフだったログイン時起動をオンにしない。
- uninstallはdisable失敗時に停止し、KeepAlive設定が削除済みバイナリを起動し続ける状態を作らない。
- 再起動でvaultが施錠されることをREADMEとinstall出力で明示する。

## テスト

- CLIルーティング: `service refresh/disable` がalias接続、ブラウザ、Webサーバーを開始しない。
- service refresh: 無効時はno-op、有効時だけ自身の絶対パスでEnableし、失敗を非zeroで返す。
- service disable: 無効時はno-op、有効時だけDisableし、失敗を非zeroで返す。
- Linuxでunitだけが残り`systemctl`を使えない場合、refresh/disableが成功扱いにならず設定ファイルも勝手に削除しない。
- macOS: refreshでbootout/bootstrapと新しいProgramArgumentsを確認する。
- Linux: 初回と既存サービスの両方でdaemon-reload、enable、restartを確認する。
- Makefile smoke: 一時INSTALL_DIRへinstallし、実行可能な同一バイナリが置かれること、無効なサービスを勝手に有効化しないこと、uninstallで削除されることを確認する。
- 回帰: `make verify-generated`、`make test`、`make e2e`、Docker統合を実行し、パッケージ・lockfileに差分がないことを確認する。

## 完了条件

- `make install` 後、`command -v sshc`、有効なLaunchAgent/systemd unit、実際の常駐プロセスがすべて安定パスを指す。
- インストールされたバイナリとproduction buildのSHA-256が一致する。
- 既に有効だったサービスは新しいバイナリで再起動し、施錠状態になる。
- ログイン時起動が無効だった環境では登録ファイルもプロセスも新規作成されない。
- `make uninstall` 後にログインサービス、インストール済みバイナリ、壊れたKeepAlive設定が残らない。

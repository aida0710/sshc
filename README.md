# sshc

sshc は、OpenSSH の設定を管理し、SSH 接続を実行するためのアプリケーションです。`~/.ssh/config` を独自形式へ変換せず、コメント、記述順、空白を保ったまま編集します。

バックエンドは `sshc engine` で起動し、Web UI と API を提供します。`sshc` を実行すると、起動中のエンジンの URL を表示し、利用可能な環境ではブラウザを開きます。

## インストール

macOS と Linux では Homebrew を利用できます。

```sh
brew install aida0710/tap/sshc
```

または、取得するスクリプトとバイナリを同じリリースへ固定して実行します。

```sh
SSHC_VERSION=v0.16.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.16.0/install.sh | sh'
```

`main` 上の可変なスクリプトを直接実行せず、導入したい[リリース](https://github.com/aida0710/sshc/releases)のタグをURLと`SSHC_VERSION`の両方へ指定してください。

Windows を含む各 OS 向けのバイナリと Android APK は、[GitHub Releases](https://github.com/aida0710/sshc/releases) からダウンロードできます。デスクトップアプリ、インストーラ、macOS の app bundle は配布していません。

Homebrewまたはreceipt対応版の`install.sh`で導入したsshcは、同じ管理元へ処理を委ねて更新できます。手動配置やソースビルドは自動で置き換えません。

```sh
sshc update
```

## 基本的な使い方

最初にエンジンを起動し、別のターミナルから UI を開きます。

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URL を表示し、可能であればブラウザで開く
```

`sshc engine` はデーモン化しません。常駐させる場合は tmux、systemd、launchd などのプロセス管理機能を使用してください。

sshc はログイン時の自動起動を設定せず、systemd unit やスケジュールタスクも作成しません。自動起動は OS のプロセス管理機能で設定します。`sshc engine` をフォアグラウンドプロセスとして登録してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時は、資格情報を保存する vault を作成します。

```sh
sshc vault create
sshc vault unlock
```

マスターパスワードは Web UI または `sshc vault` から入力できます。CLI は対話端末からのみパスワードを読み取り、引数や環境変数からは受け取りません。

SSH 接続は CLI から直接開始することもできます。

```sh
sshc <接続先>                 # 保存済みの設定を使用して接続
sshc run <接続先> <コマンド> # リモートコマンドを実行
sshc connect                  # 一覧から接続先を選択
sshc list                     # Host alias の一覧を表示
sshc status                   # エンジンの状態を表示（--json に対応）
sshc update                   # Homebrew／install.sh経由の導入を更新
```

引数なしの `sshc` はエンジンを起動しません。実行中のエンジンを置き換えるには `sshc engine --replace` を使用します。

## 主な機能

- 横断検索: desktop toolbarまたは`Ctrl/Cmd+K`から、保存済みhost、SSH設定file、snippet、設定画面を一つのCommand Paletteで検索して開けます。mobileではhamburger menu内から開き、terminalの表示領域を常時消費しません。
- OpenSSH 互換の設定管理: コメント、記述順、空白を保ったまま編集し、外部変更との競合を検出します。
- ブラウザ内ターミナル: ポート転送、エージェント転送、未知のホスト鍵の確認、切断時の自動再接続と終了後の明示再接続、`ProxyJump`、`ProxyCommand` に対応します。明示再接続は同じpaneとscrollbackを残したまま新しいshellを開きます。接続・再接続・終了を区別して表示し、利用者の確認が必要なホスト鍵や認証の問題は無条件に再試行しません。scrollback検索、live session内のcommand履歴、頻度順command候補、absolute remote path補完も利用できます。
- SFTP ファイル操作: 保存済み接続を使ったリモート閲覧、ファイル／フォルダの選択と Drag & Drop による再帰アップロード、ファイルdownload、フォルダのZIP download、作成、名前変更、chmod、削除に対応します。upload／download共通のTransfer Managerが同時2件までを実行し、file単位の進捗・速度・残り時間、pause／resume／retry／cancel、folder batchの失敗fileだけの再実行、別画面でのbackground転送と完了／失敗通知を提供します。large uploadは1 MiB chunkとremote part fileで通信断後のoffsetから再開してatomic renameし、file downloadはHTTP Rangeで再開します。UTF-8 の 2 MiB 以下のファイルは、遅延ロードされる Monaco Editor で競合を検出しながら編集できます。
- Workspace: 接続済みSSHターミナルまたはローカルシェルを、表示中のターミナルの上下左右へDrag & Dropすると、接続を増やさずLive Workspaceとして分割し、接続一覧でも1つのグループにまとめます。区切りをDragして比率を変更でき、単一paneへ集中するFocus Modeを利用できます。スマホでは分割を小さく並べず、Workspace内のターミナルをタブで1画面ずつ切り替えます。pane 構成は端末内に保存でき、Homeの一覧から1回の明示操作で再オープンできます。再オープン時は各SSH接続とローカルシェルを新しく開始し、一部が失敗しても成功したpaneを維持します。Command Centerはpreview後、接続中のSSH paneだけを対象に現在の入力先へcommandとEnterを送り、paneごとのcwd・環境・shell状態をそのまま使います。
- Snippets と automation: `{{variable}}` を含むコマンドを保存し、接続先と展開後コマンドを確認してから複数ホストへ実行できます。明示的に選んだ snippet は、SSH shell の準備完了後に startup command として送信できます。
- 接続ログ: `ssh -v` 相当の情報を、4 段階の詳細度でターミナルに表示します。
- 表示設定: 6 種類のカラーパレット、同梱の JetBrains Mono、背景画像を全体または接続ごとに設定できます。
- SSH 鍵管理: 鍵の生成、パスフレーズ変更、ssh-agent への登録、リモートの `authorized_keys` への公開鍵追加に対応します。
- 資格情報 vault: パスワードと鍵パスフレーズをマスターパスワードで暗号化します。12 時間操作がない場合は自動的にロックされます。ブラウザ向け API は保存値を返しません。CLI で接続するときは、その接続経路に必要な資格情報だけを、ローカルの handoff secret で認証した経路から CLI へ渡します。
- S3 互換バックアップ: 暗号化したスナップショットを保存し、別の端末で復元できます。

Workspace の layout はプロセス状態や session ID を保存せず、この端末だけの `~/.ssh/sshc/workspaces.json` に置かれます。Snippets は資格情報と同じ暗号化 snapshot の対象ですが、secret 型の入力値は保存されず、startup automation にも使用できません。SFTP や複数ホスト実行は対話プロンプトを出さないため、vault の保存済み資格情報と既知のホスト鍵が必要です。

設計上の判断と対象外の機能については、[設計概要](docs/design.md)を参照してください。

## 開発

Go 1.26 と Node.js 22 が必要です。

```sh
make build       # Web UI をビルドし、bin/sshc を生成
make test        # Go と Web UI のテストを実行
make e2e         # 実バイナリに対する Playwright テストをローカル実行
make generate    # OpenAPI から Go と TypeScript のコードを再生成
```

S3 互換ストレージと OpenSSH に対する統合テストでは、先に Docker コンテナを起動します。

```sh
make integration-up
make integration
make integration-down
```

`internal/ui/dist` はリポジトリに含まれ、Go バイナリへ埋め込まれます。`web` 以下を変更した場合は `make build` を実行し、生成物もコミットしてください。CI でソースとの不一致を検出します。

Android AAR の生成には NDK が必要です。

```sh
go install golang.org/x/mobile/cmd/gobind
make android-bind
```

関連資料:

- [設計概要](docs/design.md)
- [手動受け入れ試験](docs/manual-acceptance.md)
- [エンジンの常駐例](docs/headless-examples.md)
- [インストールとアップグレード](docs/release-install.md)
- [リリース履歴](docs/releases/README.md)
- [文章と用語のガイド](docs/writing-style.md)

## ライセンス

[Apache License 2.0](LICENSE)

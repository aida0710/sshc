# sshc

sshc は、OpenSSH の設定を管理し、SSH 接続を実行するためのアプリケーションです。`~/.ssh/config` を独自形式へ変換せず、コメント、記述順、空白を保ったまま編集します。

バックエンドは `sshc engine` で起動し、Web UI と API を提供します。`sshc` を実行すると、起動中のエンジンの URL を表示し、利用可能な環境ではブラウザを開きます。

## インストール

macOS と Linux では Homebrew を利用できます。

```sh
brew install aida0710/tap/sshc
```

または、インストールスクリプトを実行します。

```sh
curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/main/install.sh | sh
```

Windows を含む各 OS 向けのバイナリと Android APK は、[GitHub Releases](https://github.com/aida0710/sshc/releases) からダウンロードできます。デスクトップアプリ、インストーラ、macOS の app bundle は配布していません。

0.3.x 以前のデスクトップアプリから移行する場合は、古い cask と実行ファイルを削除してください。手順は[インストールガイド](docs/release-install.md#03x-からの移行)にあります。

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
```

引数なしの `sshc` はエンジンを起動しません。実行中のエンジンを置き換えるには `sshc engine --replace` を使用します。

## 主な機能

- OpenSSH 互換の設定管理: コメント、記述順、空白を保ったまま編集し、外部変更との競合を検出します。
- ブラウザ内ターミナル: ポート転送、エージェント転送、未知のホスト鍵の確認、切断時の再接続、`ProxyJump`、`ProxyCommand` に対応します。
- 接続ログ: `ssh -v` 相当の情報を、4 段階の詳細度でターミナルに表示します。
- 表示設定: 6 種類のカラーパレット、同梱の JetBrains Mono、背景画像を全体または接続ごとに設定できます。
- SSH 鍵管理: 鍵の生成、パスフレーズ変更、ssh-agent への登録、リモートの `authorized_keys` への公開鍵追加に対応します。
- 資格情報 vault: パスワードと鍵パスフレーズをマスターパスワードで暗号化します。12 時間操作がない場合は自動的にロックされます。ブラウザ向け API は保存値を返しません。CLI で接続するときは、その接続経路に必要な資格情報だけを、ローカルの handoff secret で認証した経路から CLI へ渡します。
- S3 互換バックアップ: 暗号化したスナップショットを保存し、別の端末で復元できます。

設計上の判断と対象外の機能については、[設計概要](docs/design.md)を参照してください。

## 開発

Go 1.26 と Node.js 22 が必要です。

```sh
make build       # Web UI をビルドし、bin/sshc を生成
make test        # Go と Web UI のテストを実行
make e2e         # 実バイナリに対する Playwright テストを実行
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

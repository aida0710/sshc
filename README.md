# sshc

OpenSSHの設定ファイルをそのまま使う、ローカルファーストのSSHクライアントです。接続管理、Terminal、SFTP、Workspace、スニペット、暗号化同期を1つのUIとCLIから扱えます。

[日本語ドキュメント](https://aida0710.github.io/sshc/) · [English documentation](https://aida0710.github.io/sshc/en/) · [Releases](https://github.com/aida0710/sshc/releases)

[![sshcのHome画面](docs/images/home.png)](https://aida0710.github.io/sshc/)

## インストール

macOS / Linux:

[Homebrew](https://brew.sh/)を未導入の場合は、公式サイトの手順でインストールします。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。

```sh
SSHC_VERSION=v0.39.2 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.39.2/install.sh | sh'
```

Windows PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

Android APKと各OS向けのバイナリは[GitHub Releases](https://github.com/aida0710/sshc/releases)から取得できます。詳しい検証方法と更新手順は[インストールガイド](https://aida0710.github.io/sshc/guide/install)を参照してください。

## 起動

```sh
sshc engine
sshc
```

`sshc engine`はフォアグラウンドで動作し、デーモン化しません。引数なしの`sshc`はエンジンを起動しません。自動起動はOSのプロセス管理機能で設定します。Linuxでは`sshc service install`でsystemdユーザーサービス、macOSではlaunchdユーザーエージェントへ登録できます。その他の環境ではtmuxなどを利用できます。

初回はWeb UIまたはCLIでVaultを作成し、ロックを解除します。

```sh
sshc vault create
sshc vault unlock
```

マスターパスワードはWeb UIまたは`sshc vault`で入力します。CLIでは対話ターミナルからの入力だけを受け付け、引数や環境変数からは受け取りません。

Vaultは既定で、Vault内のシークレットを最後に読み書きしてから12時間後に自動でロックされます。Settingsで1〜999分／時間に変更するか、自動ロックを無効にできます。

## CLIコマンド
```sh
sshc ssh                     # 接続先を選択
sshc ssh <接続先>            # 対話SSH
sshc ssh <接続先> --non-interactive -- <コマンド>
sshc info <接続先> --json    # エンジンなしで実際に使われる設定を表示
sshc status --json           # エンジンの状態
sshc sync                    # 同期状態
sshc sftp get <接続先> /remote/file ./local-file
sshc sftp put <接続先> ./local-file /remote/file
sshc help                    # すべてのコマンド
```

## 主な機能

- OpenSSHのコメント、順序、空白、`Include`を保った設定管理
- 接続状態、検索、出力を引き継ぐ再接続、貼り付け前の確認、文字コード、Quick Commandsを備えたTerminal
- フォルダ転送、中断からの再開、バックグラウンドの転送キュー、エディタを備えたSFTP
- SSHとローカルシェルを最大4ペインに並べるWorkspace
- ホスト、ファイル、スニペット、設定を横断検索するCommand Palette
- S3互換ストレージを使った暗号化スナップショット同期
- SSH、SFTP、Serial、Telnet、同期、Terminal操作のCLI

機能とセキュリティ上の制限は[利用者向けドキュメント](https://aida0710.github.io/sshc/)にまとめています。内部設計は[docs/design.md](docs/design.md)を参照してください。

## 開発

Go 1.26とNode.js 22が必要です。

```sh
make build
make test
make e2e
make integration-up
make integration
make integration-down
```

Web UIを変更したときは`internal/ui/dist`も更新してください。利用者向けサイトは`pages/`にあり、`npm run build`で検証できます。

## License

[Apache License 2.0](LICENSE)

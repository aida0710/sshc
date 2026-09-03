# インストールとアップグレード

sshc は、macOS、Linux、Windows 向けの CLI バイナリと Android APK を配布しています。デスクトップアプリ、macOS の app bundle、AppImage、パッケージ形式のWindowsインストーラは配布していません。

## Homebrew（macOS / Linux）

```sh
brew install aida0710/tap/sshc
```

Homebrew formula はソースからビルドするため、Go toolchain も Homebrew によってインストールされます。

## インストールスクリプト（macOS / Linux）

```sh
SSHC_VERSION=v0.27.1 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.27.1/install.sh | sh'
```

URLと`SSHC_VERSION`には同じ導入対象のタグを指定します。`main`上のスクリプトは次の変更で内容が変わるため、pipeで直接実行しません。新しい版へ更新するときは、[GitHub Releases](https://github.com/aida0710/sshc/releases)でタグを確認して両方を置き換えます。

スクリプトは次の項目を確認してからバイナリを配置します。

- OS と CPU アーキテクチャに対応するバイナリが公開されていること
- ダウンロードしたファイルの SHA-256 が `checksums.txt` と一致すること
- インストール先のディレクトリへ書き込めること
- 既存のインストール先がシンボリックリンクではないこと

ダウンロード、チェックサム、書き込み権限の確認に失敗した場合、既存の実行ファイルを変更せず終了します。通常は `~/.local/bin` にインストールし、root で実行した場合は `/usr/local/bin` を使用します。

GitHub CLIを使用できる場合は、配置前にartifact provenanceも検証できます。`<downloaded-file>`にはCLIまたはAPKの実ファイルを指定します。

```sh
gh attestation verify <downloaded-file> --repo aida0710/sshc
```

この検査は、SHA-256による転送破損の検出に加え、そのdigestが`aida0710/sshc`のRelease workflowで生成されたことをGitHubの署名済みattestationから確認します。

インストール後、配置先が `PATH` に含まれていない場合や、別の `sshc` が先に解決される場合は警告と設定例を表示します。`PATH` は自動変更しません。実行中のエンジンと新しい CLI のバージョンが異なる場合も、置き換え前に警告します。

インストール先は `SSHC_INSTALL_DIR`、バージョンは `SSHC_VERSION` で変更できます。

新しい`install.sh`は配置先の隣に、導入元、バージョン、配置したバイナリのSHA-256を含むreceiptを原子的に保存します。`sshc update`はこのdigestが現在の実行ファイルと一致するときだけ、公開済みtagに固定した`install.sh`へ更新を委ねます。receiptのない旧installer、手動コピー、`make install`、変更済みバイナリを推測で置き換えません。旧installerから移行する場合は、上記のtag固定手順を一度手動で実行してください。

## 自動判定による更新

```sh
sshc update
```

更新するバージョン、実行ファイル、管理元を表示した後に確認を求めます。CIなど対話端末のない環境で実行する場合は、内容を確認した上で`sshc update --yes`を使用してください。

- Homebrew版は、`brew --prefix --installed aida0710/tap/sshc`の`bin/sshc`と実行中ファイルが同一であることを確認し、`brew upgrade --formula --no-ask aida0710/tap/sshc`を実行します。
- `install.sh`版はdigest付きreceiptを確認し、GitHubの最新安定版tagに固定したinstallerを実行します。installerは公開された`checksums.txt`でバイナリを検証し、同一ディレクトリ内のrenameで置換します。
- `install.ps1`版は、インストール時と同じPowerShellコマンドを再実行して更新します。
- その他のWindows手動配置、ソースビルド、判定不能な導入は変更せず、元の導入方法で更新するよう表示します。

`sshc service install`で作成したユーザーサービスがactiveで、登録した実行パスが今回の更新対象と一致する場合だけ、更新後に再起動します。Linuxではsystemdの`try-restart`、macOSではlaunchdの`kickstart -k`を使用します。再起動後はvaultがロックされるため、対話端末から`sshc vault unlock`を実行してください。binary更新後の再起動だけに失敗した場合は、表示される`sshc service install`を実行して復旧できます。停止中のサービス、sshc管理外の定義、別のsshc導入を指す定義、サービス管理外のengineは自動で起動、再起動しません。後者は更新後に停止し、新しい`sshc engine`を起動してください。

## Windows

Windows PowerShellから、GitHub Releaseに添付されたスクリプトを実行します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

スクリプトはx64／Arm64を判定し、対応するバイナリと`checksums.txt`を同じGitHub Releaseから取得します。SHA-256が一致し、バイナリ自身が想定したWindows版・CPU・バージョンを報告した場合だけ、`%LOCALAPPDATA%\Programs\sshc\sshc.exe`を置き換えます。管理者権限は要求せず、ユーザー`PATH`へ配置先を重複なく追加します。新しいターミナルから`sshc version`で確認できます。

既存の`sshc.exe`を使用中で置換できない場合は、動作中のengineを停止してから同じコマンドを再実行してください。検証や置換に失敗した場合、既存の実行ファイルは変更しません。配置先は`SSHC_INSTALL_DIR`で変更でき、`SSHC_ADD_TO_PATH=0`なら`PATH`を変更しません。

再現可能な導入では、スクリプトと成果物を同じタグへ固定します。

```powershell
$env:SSHC_VERSION = 'v0.27.1'
irm https://github.com/aida0710/sshc/releases/download/v0.27.1/install.ps1 | iex
```

手動で配置する場合は、[GitHub Releases](https://github.com/aida0710/sshc/releases) からx64では`sshc-windows-amd64.exe`、Arm64では`sshc-windows-arm64.exe`を取得し、`checksums.txt`と照合してから`sshc.exe`へ名前を変更します。

`install.ps1`はシステム領域やシステム`PATH`を変更しません。Windowsの「インストール済みアプリ」へ登録するMSI／MSIX／EXEインストーラではありません。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases) から `sshc-android-v<version>.apk` をダウンロードしてください。APK はリリース用の固定した証明書で署名され、Release workflowは公開前に証明書のSHA-256 fingerprintを照合します。

## 起動

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URL を表示し、可能であればブラウザで開く
```

`sshc engine` はデーモン化しません。Homebrew版またはreceipt対応`install.sh`版は、Linuxではsystemdユーザーサービス、macOSではlaunchdユーザーエージェントへ`sshc service install`で登録できます。その他の環境ではtmuxやscreenなどを手動で使用してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時は vault を作成してロックを解除します。

```sh
sshc vault create
sshc vault unlock
```

同じ操作は Web UI からも実行できます。CLI は対話端末からのみマスターパスワードを読み取り、コマンドライン引数や環境変数からは受け取りません。

vault は既定では 12 時間操作がない場合に自動的にロックされます。Settingsで1〜999分／時間、または自動ロックなしへ変更できます。

## リポジトリ管理者向けの公開保護

workflow内の検査だけでは、公開後のasset変更や管理者資格情報の侵害を止められません。リポジトリ設定で次を維持します。

- GitHub Immutable Releasesを有効にし、公開済みReleaseのassetと本文を変更不能にする
- `release` environmentにrequired reviewerを設定し、administrator bypassを無効にする。単独管理者の間は公開不能を避けるためself reviewを許可するが、公開ごとの明示承認は必須とする。別の管理者を置ける場合はself reviewも無効にする
- `main`はstrictなrequired CI 9件、review、linear history、conversation resolutionを要求し、force pushと削除を禁止する。単独管理者の間は直接push後の同一SHA CIをrelease source gateで必ず検証し、別の管理者を置ける場合はadministrator enforcementも有効にする
- `v*` tagの更新と削除を禁止し、bypass actorを設定しない

これらはrepository外の設定であり、workflowのcommitだけでは有効になりません。公開前監査ではGitHub APIまたはSettings画面で現在値を確認します。

## バージョン不一致

CLI と実行中のエンジンのバージョンが異なる場合、sshc は接続せず、現在使用している実行ファイルのパスを表示します。エンジンを停止し、更新後の `sshc engine` で起動し直してください。

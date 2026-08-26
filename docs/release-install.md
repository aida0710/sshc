# インストールとアップグレード

sshc は、macOS、Linux、Windows 向けの CLI バイナリと Android APK を配布しています。デスクトップアプリ、macOS の app bundle、AppImage、Windows インストーラは配布していません。

## Homebrew（macOS / Linux）

```sh
brew install aida0710/tap/sshc
```

Homebrew formula はソースからビルドするため、Go toolchain も Homebrew によってインストールされます。

## インストールスクリプト（macOS / Linux）

```sh
SSHC_VERSION=v0.15.3 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.15.3/install.sh | sh'
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

- Homebrew版は、`brew --prefix --installed aida0710/tap/sshc`の`bin/sshc`と実行中ファイルが同一であることを確認し、`brew upgrade --formula --no-ask aida0710/tap/sshc`を実行します。
- `install.sh`版はdigest付きreceiptを確認し、GitHubの最新安定版tagに固定したinstallerを実行します。installerは公開された`checksums.txt`でバイナリを検証し、同一ディレクトリ内のrenameで置換します。
- Windows、手動配置、ソースビルド、判定不能な導入は変更せず、元の導入方法で更新するよう表示します。

更新時にengineが動作していた場合、engineは旧バイナリのままです。更新後に停止し、新しい`sshc engine`を起動してください。

## Windows

[GitHub Releases](https://github.com/aida0710/sshc/releases) から、x64 では `sshc-windows-amd64.exe`、Arm64 では `sshc-windows-arm64.exe` をダウンロードしてください。ファイル名を `sshc.exe` に変更し、`PATH` に含まれるディレクトリへ配置します。

sshc は Windows インストーラを提供せず、レジストリやシステムの `PATH` を変更しません。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases) から `sshc-android-v<version>.apk` をダウンロードしてください。APK はリリース用の固定した証明書で署名され、Release workflowは公開前に証明書のSHA-256 fingerprintを照合します。

## 起動

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URL を表示し、可能であればブラウザで開く
```

`sshc engine` はデーモン化しません。常駐させる場合は tmux、screen、systemd、launchd などを使用してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時は vault を作成してロックを解除します。

```sh
sshc vault create
sshc vault unlock
```

同じ操作は Web UI からも実行できます。CLI は対話端末からのみマスターパスワードを読み取り、コマンドライン引数や環境変数からは受け取りません。

vault は 12 時間操作がない場合に自動的にロックされます。

## リポジトリ管理者向けの公開保護

workflow内の検査だけでは、公開後のasset変更や管理者資格情報の侵害を止められません。リポジトリ設定で次を維持します。

- GitHub Immutable Releasesを有効にし、公開済みReleaseのassetと本文を変更不能にする
- `release` environmentにrequired reviewerを設定し、self reviewとadministrator bypassを無効にする
- `main`のbranch protectionをadministratorにも適用し、required CIとreviewを迂回させない
- `v*` tagの更新と削除を禁止し、bypass actorを設定しない

これらはrepository外の設定であり、workflowのcommitだけでは有効になりません。公開前監査ではGitHub APIまたはSettings画面で現在値を確認します。

## バージョン不一致

CLI と実行中のエンジンのバージョンが異なる場合、sshc は接続せず、現在使用している実行ファイルのパスを表示します。エンジンを停止し、更新後の `sshc engine` で起動し直してください。

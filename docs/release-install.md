# インストールとアップグレード

sshcは、macOS、Linux、Windows向けのCLIバイナリとAndroidのAPKを配布しています。デスクトップアプリ、macOSのapp bundle、AppImage、パッケージ形式のWindowsインストーラーは配布していません。

## Homebrew（macOS / Linux）

```sh
brew install aida0710/tap/sshc
```

Homebrewのformulaはソースからビルドするため、GoもHomebrewによってインストールされます。

## インストールスクリプト（macOS / Linux）

```sh
SSHC_VERSION=v0.44.1 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.44.1/install.sh | sh'
```

URLと`SSHC_VERSION`には同じ導入対象のタグを指定します。`main`上のスクリプトは次の変更で内容が変わるため、パイプで直接実行しません。新しいバージョンへ更新するときは、[GitHub Releases](https://github.com/aida0710/sshc/releases)でタグを確認して両方を置き換えます。

スクリプトは次の項目を確認してからバイナリを配置します。

- OSとCPUアーキテクチャに対応するバイナリが公開されていること。macOSでRosetta 2のターミナルから実行した場合も、Apple SiliconのMacにはarm64版を選びます
- ダウンロードしたファイルのSHA-256が`checksums.txt`と一致すること
- 2.49.0以降のGitHub CLI（`gh`）がgithub.comにログインしている場合は、ダウンロードしたバイナリのattestationが、`aida0710/sshc`のReleaseワークフローの署名であること（下の`gh attestation verify`と同じ条件）
- ダウンロードしたバイナリが`sshc version`で、選んだOS・CPUと`SSHC_VERSION`のバージョンを報告すること
- インストール先のディレクトリへ書き込めること
- 既存のインストール先がシンボリックリンクやディレクトリではないこと

これらの確認に失敗した場合、既存の実行ファイルを変更せず終了します。attestationの検証は、署名が一致しない場合のほか、GitHubへの接続やAPIの呼び出しに失敗した場合も通らず、`gh`のエラーをそのまま表示して終了します。

次の場合は、attestationを検証せず、チェックサムだけで配置したことを表示します。

- `gh`がない
- `gh`が2.49.0より前で、`gh attestation`がない（Ubuntu 24.04のaptで入る2.45.0など）
- `gh`がgithub.comにログインしていない

`SSHC_VERSION`を指定しない場合は、タグを固定せずにReleaseワークフローの署名を確かめます。通常は`~/.local/bin`にインストールし、rootで実行した場合は`/usr/local/bin`を使用します。

手動でダウンロードしたCLIやAPKは、GitHub CLIで次のように検証できます。`<downloaded-file>`にはCLIまたはAPKの実ファイルを、`<tag>`には導入するタグ（例: `v0.44.1`）を指定します。

```sh
gh attestation verify <downloaded-file> --repo aida0710/sshc \
  --signer-workflow aida0710/sshc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag> --deny-self-hosted-runners
```

この検査は、SHA-256による転送破損の検出に加え、そのファイルを`<tag>`のpushで動いた`aida0710/sshc`のReleaseワークフロー（`.github/workflows/release.yml`）がGitHub-hosted runnerで署名したことを、GitHubの署名済みattestationから確認します。`--repo`だけでは、同じリポジトリのほかのブランチやワークフローが署名したattestationも通るため、署名したワークフローとrefも指定します。`--source-ref refs/tags/<tag>`で通らなかった場合に限り、`--source-ref refs/heads/main`でも検証します。Releaseワークフローを`main`から手動で実行して復旧したリリースは、こちらで通ります。この条件はタグと結び付かず、`main`のReleaseワークフローが作ったほかのバージョンの成果物も通ります。このため、CLIは`sshc version`が`<tag>`を表示すること、APKはインストール後にAndroidのアプリ情報に表示されるバージョンが`<tag>`から先頭の`v`を除いたものであることも確かめてください。

この検査では、タグそのものが正規の手順で作られたかは確かめられません。リポジトリ設定で禁止している`v*`タグの操作は更新と削除だけで、作成は含みません。新しいタグを導入するときは、次のコマンドが`ahead`または`identical`を返すこと（タグのコミットが`main`に含まれていること）も確認してください。

```sh
gh api repos/aida0710/sshc/compare/<tag>...main --jq .status
```

インストール後、配置先が`PATH`に含まれていない場合や、別の`sshc`が先に見つかる場合は警告と設定例を表示します。`PATH`は自動変更しません。実行中のエンジンと新しいCLIのバージョンが異なる場合は、置き換え前に警告し、再起動の方法を表示します（下の「[エンジンの再起動](#エンジンの再起動)」）。

インストール先は`SSHC_INSTALL_DIR`、バージョンは`SSHC_VERSION`で変更できます。

現在の`install.sh`は、配置先と同じディレクトリに、導入元、バージョン、配置したバイナリのSHA-256を記録したreceiptを保存します。前のreceiptを削除してからバイナリを置き、最後に新しいreceiptを置くため、途中で止まってもreceiptが別のバイナリを指すことはありません。途中で止まった場合はreceiptのない導入になるので、同じ手順を実行し直してください。

`sshc update`は、receiptのSHA-256が現在の実行ファイルと一致するときだけ、公開済みのタグに固定した`install.sh`で更新します。receiptを書かない古いインストーラー、手動コピー、`make install`で入れたもの、変更したバイナリは、推測で置き換えません。SHA-256が一致しない場合、`sshc update`と`sshc service install`はエラーで終了します。`install.sh`で入れ直すか、実行ファイルを自分で置き換えた場合はreceiptを削除してください。古いインストーラーから移行する場合は、上のタグを固定した手順を一度手動で実行してください。

`SSHC_VERSION`でプレリリース（`v0.44.1-rc.1`のように`-`を含むタグ）を指定した場合、receiptは保存せず、同じ配置先にある前のreceiptも削除します。このため、プレリリースは`sshc update`と`sshc service install`の対象外です。対象に戻すには、安定バージョンのタグを指定して`install.sh`を実行し直してください。

## 自動判定による更新

```sh
sshc update
```

更新するバージョン、実行ファイル、管理元を表示した後に確認を求めます。CIなど対話型のターミナルのない環境で実行する場合は、内容を確認した上で`sshc update --yes`を使用してください。

- Homebrew版は、`brew --prefix --installed aida0710/tap/sshc`の`bin/sshc`と実行中ファイルが同一であることを確認し、`brew upgrade --formula --no-ask aida0710/tap/sshc`を実行します。Homebrewがtapを更新しなかったために新しいバージョンが入らなかった場合（`HOMEBREW_NO_AUTO_UPDATE`を設定している場合など）は、`brew update`を実行してから`sshc update`を再実行するよう表示します。
- `install.sh`版は、SHA-256を記録したreceiptを確認し、GitHubの最新の安定バージョンのタグに固定した`install.sh`を実行します。`install.sh`は公開された`checksums.txt`でバイナリを検証し、同じディレクトリの中で名前を変更して置き換えます。Ctrl-Cなどで取り消すと、`install.sh`に一時ファイルを削除させてから停止します。
- Windows（`install.ps1`版を含む）では`sshc update`は使えず、終了コード1で終わります。そのときに、インストール時と同じPowerShellコマンドを表示します。このコマンドを手で再実行して更新してください。エンジンが起動時に新しいバージョンを知らせるときも、Windowsではこのコマンドを表示します。
- Windows以外の手動配置、ソースビルド、判定不能な導入は変更せず、元の導入方法で更新するよう表示します。

`sshc service install`で作成したユーザーサービスが動作中で、登録した実行パスが今回の更新対象と一致する場合だけ、更新後に再起動します。Linuxではsystemdの`try-restart`、macOSではlaunchdの`kickstart -k`を使用します。パスワードを設定したVaultは再起動でロックされるため、ターミナルから`sshc vault unlock`を実行してください。パスワードなしのVaultは、エンジンの起動時に自動でロックを解除します。バイナリの更新後に再起動だけが失敗した場合は、表示される`sshc service install`を実行して復旧できます。停止中のサービス、sshc管理外の定義、別のsshc導入を指す定義、サービス管理外のエンジンは、自動では起動も再起動もしません。

### エンジンの再起動

更新後のバージョンを使うには、動いているエンジンを再起動します。

- `sshc service install`で登録したサービス: `sshc update`で更新した場合は、上のとおり自動で再起動します。`install.sh`を直接実行した場合など、稼働中のサービスを手動で再起動するときは`sshc service restart`を実行します。停止中や古い定義の場合は`sshc service install`を使用します
- それ以外のエンジン（フォアグラウンドやtmuxで起動したもの）: `sshc engine --replace`で、更新後のバイナリから起動し直します

`sshc service restart`はLinuxとmacOSで利用できます。この導入の安定パスと現在のサービス定義が一致する場合だけ、稼働中のサービスを再起動します。接続中のセッションと転送が終了するため、対象を表示して確認を求めます。確認を省略する場合は`-y`または`--yes`を指定します。未登録・停止中・管理外・古い定義・別の実行ファイルを指すサービスは拒否し、終了コード1で終わります。定義の書き換えや再登録は行いません。

## Windows

Windows PowerShellから、GitHub Releaseに添付されたスクリプトを実行します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

スクリプトはx64／Arm64を判定し、対応するバイナリと`checksums.txt`を同じGitHub Releaseから取得します。SHA-256が一致し、バイナリ自身が想定したWindows版・CPU・バージョンを報告した場合だけ、`%LOCALAPPDATA%\Programs\sshc\sshc.exe`を置き換えます。管理者権限は要求せず、ユーザー`PATH`へ配置先を重複なく追加します。新しいターミナルから`sshc version`で確認できます。

既存の`sshc.exe`を使用中で置換できない場合は、動作中のエンジンを停止してから同じコマンドを再実行してください。検証や置換に失敗した場合、既存の実行ファイルは変更しません。配置先は`SSHC_INSTALL_DIR`で変更でき、`SSHC_ADD_TO_PATH=0`なら`PATH`を変更しません。

再現可能な導入では、スクリプトと成果物を同じタグへ固定します。

```powershell
$env:SSHC_VERSION = 'v0.44.1'
irm https://github.com/aida0710/sshc/releases/download/v0.44.1/install.ps1 | iex
```

手動で配置する場合は、[GitHub Releases](https://github.com/aida0710/sshc/releases)からx64では`sshc-windows-amd64.exe`、Arm64では`sshc-windows-arm64.exe`を取得し、`checksums.txt`と照合してから`sshc.exe`へ名前を変更します。

`install.ps1`はシステム領域やシステム`PATH`を変更しません。Windowsの「インストール済みアプリ」へ登録するMSI／MSIX／EXEのインストーラーではありません。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases)から`sshc-android-v<version>.apk`をダウンロードしてください。APKはリリース用の固定した証明書で署名され、Releaseワークフローは公開前に証明書のSHA-256フィンガープリントを照合します。

## 起動

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URLを表示し、可能であればブラウザで開く
```

`sshc engine`はデーモン化しません。Homebrew版と、receiptに対応した`install.sh`版は、Linuxではsystemdユーザーサービス、macOSではlaunchdユーザーエージェントへ`sshc service install`で登録できます。その他の環境ではtmuxやscreenなどを手動で使用してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時はVaultを作成し、ロックを解除します。

```sh
sshc vault create
sshc vault unlock
```

同じ操作はWeb UIからも実行できます。CLIは対話型のターミナルからのみマスターパスワードを読み取り、コマンドライン引数や環境変数からは受け取りません。

パスワードを設定したVaultは既定で、Vault内のシークレットを最後に読み書きしてから12時間後に自動でロックされます。Settingsで1〜999分／時間に変更するか、自動ロックを無効にできます。パスワードなしのVaultは自動ではロックされず、エンジンの起動時に自動でロックを解除します。

## アンインストール

常駐させているエンジンを先に止め、サービスの登録を削除してから、実行ファイルを削除します。サービスに登録したまま実行ファイルだけを削除すると、macOSのlaunchdは5秒ごとにエンジンの起動を試みて失敗し続けます。Linuxのsystemdでも、ログインのたびに起動に失敗します。

1. `sshc service install`で登録している場合は、`sshc service disable`を実行します。sshcが作成したサービス定義だけを停止、無効化、削除します。フォアグラウンドやtmuxで起動したエンジンは、そのプロセスを停止します。
2. 導入方法に合わせて実行ファイルを削除します。
   - Homebrew: `brew uninstall aida0710/tap/sshc`
   - `install.sh`: 配置先の`sshc`と、同じディレクトリにあるreceipt（`.sshc-install-receipt.json`）を削除します。配置先は通常`~/.local/bin`で、rootで実行した場合は`/usr/local/bin`、`SSHC_INSTALL_DIR`を指定した場合はそのディレクトリです。
   - `make install`: `make uninstall`で`~/.local/bin/sshc`を削除します。
   - Windows: 動いているエンジンを停止してから、下のコマンドで`sshc.exe`を削除し、ユーザー`PATH`から配置先を外します。
   - Android: ほかのアプリと同じ手順でアンインストールします。
3. 接続ごとのVPNを使った場合は、sshcが作成したコンテナイメージ`sshc-vpn`が残ります。VPNのコンテナはエンジンの停止時に削除されます。イメージが不要なら、`docker image ls sshc-vpn`で確認してから`docker image rm`で削除してください。

Windowsでは、PowerShellで次を実行します。`SSHC_INSTALL_DIR`で配置先を変えた場合は、`$dir`にそのディレクトリを指定してください。

```powershell
$dir = Join-Path $env:LOCALAPPDATA 'Programs\sshc'
Remove-Item -LiteralPath (Join-Path $dir 'sshc.exe')
$entries = [Environment]::GetEnvironmentVariable('Path', 'User') -split ';' |
  Where-Object { $_ -and [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ine $dir }
[Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
```

`sshc service disable`を実行する前に実行ファイルを削除した場合は、次のコマンドでサービス定義を削除します。

```sh
# Linux
systemctl --user disable --now sshc
rm ~/.config/systemd/user/sshc.service
systemctl --user daemon-reload

# macOS
launchctl bootout gui/$(id -u)/io.github.aida0710.sshc
rm ~/Library/LaunchAgents/io.github.aida0710.sshc.plist
```

sshcのデータは、アンインストールしても`~/.ssh/sshc`（Windowsでは`%USERPROFILE%\.ssh\sshc`）に残ります。Vault、接続先ごとのsshcの設定、VPNプロファイルの設定、SSH設定を変更する前のバックアップ、削除した鍵を含みます。使い直す予定がなく、中身が不要なことを確認した場合だけ、このディレクトリを削除してください。削除したVaultとバックアップは復元できません。`~/.ssh/config`、`Include`先のファイル、鍵はOpenSSHと共有しているため、sshcをアンインストールしても変更しません。暗号化同期でS3互換ストレージへ保存したスナップショットも残るため、不要な場合はストレージ側で削除してください。

## リポジトリ管理者向けの公開保護

ワークフローの中の検査だけでは、公開後にアセットを差し替える操作や、管理者の認証情報の侵害を止められません。リポジトリの設定で次を維持します。

- GitHub Immutable Releasesを有効にし、公開済みReleaseのアセットと本文を変更できないようにする
- `release` environmentにrequired reviewerを設定し、administrator bypassを無効にする。単独管理者の間は、公開できなくなるのを避けるためself reviewを許可する。別の管理者を置ける場合はself reviewも無効にする
- 単独管理者の間は、管理者が`scripts/release/publish.sh`を実行したことを、その公開の承認とみなす。スクリプトは、タグのpushで始まったReleaseワークフローのrunについて、実行した管理者のghの認証情報で`release` environmentだけを承認する。publish.sh以外から始まったrun（手動でpushしたタグ、`workflow_dispatch`による作り直し）は、管理者がrunの画面で承認するまで止まる
- `main`はstrictなrequired CI 9件、review、linear history、conversation resolutionを要求し、force pushと削除を禁止する。単独管理者の間は、直接pushしたコミットと同じSHAのCIをrelease source gateで必ず検証し、別の管理者を置ける場合はadministrator enforcementも有効にする
- `v*`タグの更新と削除を禁止し、bypass actorを設定しない

これらはリポジトリのファイルの外にある設定で、ワークフローをコミットしただけでは有効になりません。公開前の監査では、GitHub APIまたはSettings画面で現在の値を確認します。

## バージョン不一致

CLIは、実行中のエンジンとバージョンが違っても、通常どおり接続します。接続しないのは、CLIがエンジンを見つけるための情報（handoff）の形式か、CLIとエンジンのあいだの通信の形式が変わった組み合わせだけです。この場合は、現在使用している実行ファイルのパスを表示して終了します。

このため、更新したあとも古いエンジンが動いたままだと、新しいCLIのコマンドが古いエンジンで失敗することがあります。CLIのバージョンは`sshc version`、動いているエンジンのバージョンは`sshc status`の`version`で確認できます。違っていれば古い方を更新し、「[エンジンの再起動](#エンジンの再起動)」のとおりエンジンを起動し直してください。

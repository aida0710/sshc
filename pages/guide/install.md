---
title: インストール
description: macOS、Linux、FreeBSD、OpenBSD、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、FreeBSD、OpenBSD、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがsshcエンジン、CLI、Web UIを提供しています。

## macOS / Linux

[Homebrew](https://brew.sh/ja/)に対応しています。Homebrewを導入していない場合は、先に公式サイトの手順でHomebrewをインストールしてください。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。次は`v0.59.5`を導入する例です。

```sh
SSHC_VERSION=v0.59.5 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.59.5/install.sh | sh'
```

導入後は`sshc update`で更新できます。Homebrewで入れた場合はHomebrewから、`install.sh`で入れた場合は同じ配布元から更新されます。スクリプトなどで確認を省略する場合は、`sshc update --yes`を指定してください。

## FreeBSD / OpenBSD

FreeBSDとOpenBSD（amd64・arm64）向けのバイナリは、v0.59.1から配布しています。Homebrewは使えないので、インストールスクリプトで導入します。curlもwgetも無ければ、スクリプトはFreeBSDのベースシステムの`fetch`、OpenBSDの`ftp`で取得し、`sha256`でSHA-256を確かめます。

```sh
# FreeBSD
SSHC_VERSION=v0.59.5 sh -c \
  'fetch -qo - https://raw.githubusercontent.com/aida0710/sshc/v0.59.5/install.sh | sh'

# OpenBSD
SSHC_VERSION=v0.59.5 sh -c \
  'ftp -Vo - https://raw.githubusercontent.com/aida0710/sshc/v0.59.5/install.sh | sh'
```

`sshc update`での更新はmacOS・Linuxの`install.sh`版と同じです。`sshc service install`には対応していません。ログインしたあとも動かし続けるには、tmuxやscreenの中で`sshc engine`を実行してください。

## Windows

Windows PowerShellからインストールできます。管理者権限は不要です。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

`%LOCALAPPDATA%\Programs\sshc`へインストールされ、ユーザーの`PATH`の先頭へ追加されます。新しいターミナルで`sshc version`を実行して確認してください。別の`sshc.exe`が優先される場合は、インストーラーが場所を表示します。

### 更新する

`install.ps1`で導入したsshcは、`sshc update`とWeb画面の更新ボタンで更新できます。導入記録と実行ファイルのSHA-256が一致する場合だけ更新します。上記のインストール用コマンドを再実行しても更新できます。

更新前に動いていたsshcエンジンは、更新後に起動し直します。有効なタスクがあればタスクから起動し、それ以外はバックグラウンドで起動します。停止中だった場合は起動しません。再起動で接続中のセッションと転送は終了し、パスワードを設定したVaultはロックされます。

### ログオン時に起動する

PowerShellのインストーラーで導入したsshcは、タスクスケジューラへ登録できます。管理者権限は不要です。起動中のsshcエンジンを`sshc stop`で停止してから実行してください。

```powershell
sshc service install
sshc service status
```

`install`は実行ファイルのパスを表示して確認を求めます。確認後、タスクスケジューラのルートに`sshc Startup Task-<ユーザーのSID>`という名前のタスクを登録し、その場で起動します。以後は、このユーザーがWindowsへログオンすると起動します。コンソールの窓は起動直後に閉じ、sshcエンジンは動き続けます。ログは`%USERPROFILE%\.ssh\sshc\logs\engine.log`へ追記します。

`sshc service status`はタスクの状態（Ready／Running／Disabled）とsshcエンジンの応答を表示します。稼働中のタスクは`sshc service restart`で再起動できます。`sshc service disable`はsshcエンジンの終了を待ち、タスクを削除します。変更操作の確認を省略する場合は`--yes`を付けてください。

同名の手作りのタスクは上書きも削除もしません。ダウンロードしたexeの直接実行やソースビルドは登録できません。インストーラーがexeの隣に置く`.sshc-install-receipt.json`で導入元とSHA-256を確認します。この導入記録は更新時にも確認します。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases)から`sshc-android-v<version>.apk`をダウンロードできます。APKはReleaseワークフローで署名フィンガープリントとチェックサムを検査してから公開しています。

起動方法やファイル選択の動作は、[Android](/platform/android)で詳しく説明しています。

## 初回起動

```sh
sshc
```

`sshc`と`sshc open`は、sshcエンジンが停止中ならバックグラウンドで起動します。準備ができると一度限りのURLを表示し、ブラウザで開きます。Windowsを含め、ターミナルを閉じてもsshcエンジンは動き続けます。ブラウザを開かずURLだけを表示する場合は、`sshc open --no-browser`を使います。

停止するには`sshc stop`を実行します。このターミナルで動かす場合は`sshc engine`を使ってください。バックグラウンド起動のログは`~/.ssh/sshc/logs/engine.log`（Windowsは`%USERPROFILE%\.ssh\sshc\logs\engine.log`）へ追記します。このログは暗号化同期の対象外です。

デスクトップ版は、最初に`http://127.0.0.1:54447/`で待ち受けます。既定のポートを別のプロセスが使用している場合は、空いているポートを1つ選び、このマシン固有のURLとして保存します。現在のURLは`sshc status`で確認できます。

Web UI自体は、この固定URLから開けます。初めて使うブラウザを登録するときだけ、`sshc`コマンドが一度限りの登録URLを開きます。一度登録したブラウザから同じポートへ接続した場合は、sshcエンジンを再起動した後も通常は再登録を求められません。保存したポートを使用できずURLが変わった場合は、以前のブラウザ登録が失効します。現在のsshcエンジンに対して、もう一度`sshc`を実行してください。別のブラウザやブラウザプロファイルを追加する場合も同じです。

ChromeやEdgeでは、ブラウザの「アプリをインストール」からsshcをWebアプリとして追加できます。Webアプリはsshcエンジンを起動するものではありません。先に`sshc`を実行するか、OSのサービスでsshcエンジンを動かしてください。

初回起動時にVaultのマスターパスワードを設定できます。

## Ubuntu / Linuxで常駐させる

systemdを使用しているLinuxでは、ユーザーサービスをインストールできます。sudoは不要です。

起動中のsshcエンジンがある場合は、先に`sshc stop`で停止してください。同じユーザーのsshcエンジンは同時に1つしか起動できないため、既存のsshcエンジンが動いているとsystemd側の起動に失敗します。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/.config/systemd/user/sshc.service`を作成し、有効化してから、sshcエンジンが起動できたことを確認します。Homebrew版では更新後も変わらない実行ファイルパスを、`install.sh`版では検出したインストール先をサービス定義へ登録します。手動配置やソースビルドは自動登録しません。

`sshc service install`は、実行前に、作成するsystemdユーザーサービスの内容と登録する実行ファイルのパスを表示して確認を求めます。自動化で確認を省略するときは`sshc service install --yes`を使用してください。

同じパスに手作業で作成したサービス定義がある場合、sshcは上書きしません。既存のサービスを停止し、定義ファイルを退避してから実行してください。

```sh
systemctl --user disable --now sshc
mv ~/.config/systemd/user/sshc.service ~/.config/systemd/user/sshc.service.manual
systemctl --user daemon-reload
sshc service install
```

SSHログインを切断した後も起動を続ける場合は、管理者にlingerの有効化を依頼するか、権限があれば次を実行します。

```sh
loginctl enable-linger "$USER"
```

サービスを削除する場合は`sshc service disable`を実行します。このコマンドはsshcが作成したサービス定義だけを削除し、削除前に確認を求めます。自動化では`sshc service disable --yes`を使用できます。

## macOSで常駐させる

macOSでは、ユーザーエージェントとしてlaunchdへ登録できます。sudoは不要です。起動中のsshcエンジンを`sshc stop`で停止してから実行してください。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/Library/LaunchAgents/io.github.aida0710.sshc.plist`を作成し、現在のGUIユーザーへ登録してから、sshcエンジンが起動できたことを確認します。同じパスに手作業で作成したplistがある場合は上書きしません。削除は`sshc service disable`で行い、sshcが作成したplistだけを対象にします。

launchdがsshcエンジンを再起動するのは、sshcエンジンが異常終了した場合だけです。`sshc engine --replace`で置き換えた場合など、sshcエンジンが正常に終了した場合は再起動しません。サービスに戻すには`sshc service install`を再実行してください。

以前のバージョンのsshcで登録したplistは、`sshc service install`を再実行するまで、sshcエンジンが正常に終了しても再起動する以前の定義のままです。`sshc service status`、`sshc service restart`、`sshc update`は、この場合に`sshc service install`の再実行を案内します。

## 登録済みサービスを再起動する

Linux・macOS・Windowsでは、稼働中のユーザーサービスを次のコマンドで再起動できます。

```sh
sshc service restart
```

サービス定義の場所と実行ファイルのパスを表示して、再起動するか確認します。再起動すると接続中のセッションと転送が終了します。自動化で確認を省略する場合は`sshc service restart --yes`を使用してください。

このコマンドで使えるのは、Homebrew、導入記録のある`install.sh`、Windowsの`install.ps1`で導入され、この導入の安定パスと現在の定義が一致するサービスです。次の場合は再起動せず、終了コード1で終わります。

| 状態 | 対処 |
| --- | --- |
| 未登録・停止中 | `sshc service install`で登録・起動する |
| sshc管理外の定義 | 手書きの定義を確認し、そのサービスの管理方法で操作する |
| 古い定義 | `sshc service install`で現在の定義へ更新する |
| 別の実行ファイルを指す定義 | 登録した導入から実行する。現在の導入へ切り替える場合は`sshc service install`を使う |

確認待ちの間に定義や稼働状態が変わった場合も、成功扱いにはしません。`sshc service status`で状態を確認してから再実行してください。再起動後はsshcエンジンの起動情報と応答を照合します。Linux・macOSではサービスのPIDを、Windowsでは起動情報のPIDから取得した実行ファイルを確認します。パスワードを設定したVaultはロックされるため、`sshc vault unlock`を実行してください。パスワードなしのVaultは自動でロックを解除します。

## 更新

- Homebrew／`install.sh`／Windowsの`install.ps1`: `sshc update`
- Android: GitHub Releasesから新しいAPKをインストール

### Web画面から更新する

メニューのOthersにある［About sshc］、またはサイドバー左上のロゴの横にあるバージョンから、現在のバージョン・コミット・導入方法とリリースノートを確認できます。リリースノートは、取得した最新100件のうち、現在のバージョンから最新の安定版までを表示します。本文はMarkdownを整形せず、そのまま表示します。

sshcエンジンは起動時と6時間ごとに更新を確認し、結果を保持します。サイドバーは1時間ごとにその結果を読み直します。About sshcを開くと、更新処理の実行中を除き、その場でもう一度確認します。新しいバージョンが無ければ、そのことを表示します。もう一度確認するには、［更新を確認］を押してください。更新できる場合は、同じページの［sshcを更新］から進めます。ボタンの下には、確認と更新の経過が時刻付きで表示されます。確認に失敗した場合も、前回取得したリリース情報は残ります。

macOS・LinuxのHomebrew版と`install.sh`版、Windowsの`install.ps1`版では、新しい安定版があると、画面のバージョン表示欄の下にお知らせと［About sshcで更新］が出ます。押すとAbout sshcが開き、［sshcを更新］から更新します。バージョンの横の印で折りたためます。折りたたんだお知らせは、さらに新しい安定版が出るまで閉じたままです。確認画面で現在のバージョン、更新先、導入方法を確認してから実行してください。更新にはインストール先への書き込み権限が必要です。Homebrew版は`brew upgrade`で更新し、実際に入ったバージョンを確認します。実行時のHomebrewの定義に従うため、確認画面より新しいバージョンが入る場合があります。`install.sh`版と`install.ps1`版は確認したバージョンへ更新します。手動配置・開発ビルド・Androidでは、画面に表示される案内に従って更新します。

更新後はsshcエンジンが再起動し、接続中のセッションと転送が終了します。パスワードを設定したVaultは、再起動後にロックを解除してください。更新中の進行状況はAbout sshcの経過に表示されます。経過の下には、インストーラー（`brew upgrade`、`install.sh`、`install.ps1`）の出力と、sshcエンジンを再起動する経過が表示されます。出力は実行中から順に追加され、再起動の後も続きから表示されます。ページを再読み込みしても、次の更新を始めるまでは直前の更新の出力が残ります。About sshc以外の画面では、進行状況をダイアログで表示します。結果はダイアログで、1回の更新につき1回だけ表示されます。重複した更新は受け付けません。インストールが成功して再起動だけに失敗した場合は、再インストールせず、案内に従って再起動してから画面を再読み込みしてください。

### CLIから更新する

Windowsでは、[Windowsの更新](#更新する)で説明したとおり、インストーラーがsshcエンジンの停止と再起動を行います。以下はmacOS・Linuxの動作です。

`sshc service install`で管理しているサービスが動作中で、サービス定義に記録された実行ファイルが今回の更新対象と一致する場合だけ、`sshc update`が更新後にサービスを再起動します。パスワードを設定したVaultは再起動でロックされるため、`sshc vault unlock`を実行してください。パスワードなしのVaultは、sshcエンジンの起動時に自動でロックを解除します。更新は成功したものの再起動だけに失敗した場合は、表示に従って`sshc service install`を再実行できます。`install.sh`を直接実行して更新した場合は、サービスを自動では再起動しないため、稼働中なら`sshc service restart`を実行してください。停止中や古い定義の場合は`sshc service install`を使います。サービス管理外のsshcエンジンは`sshc engine --replace`で再起動します。

## アンインストール

Windowsでは、下記の削除操作で停止とサービス登録の削除をまとめて行います。macOS・Linuxでは、常駐させているsshcエンジンを先に止め、サービスの登録を削除してから、実行ファイルを削除します。サービスに登録したまま実行ファイルだけを削除すると、macOSのlaunchdは5秒ごとにsshcエンジンの起動を試みて失敗し続けます。Linuxのsystemdでも、ログインのたびに起動に失敗します。

1. `sshc service install`で登録している場合は、`sshc service disable`を実行します。sshcが作成したサービス定義だけを停止、無効化、削除します。サービス以外で起動したsshcエンジンは`sshc stop`で停止します。
2. 導入方法に合わせて実行ファイルを削除します。
   - Homebrew: `brew uninstall aida0710/tap/sshc`
   - `install.sh`: 配置先の`sshc`と、同じディレクトリにあるreceipt（`.sshc-install-receipt.json`）を削除します。配置先は通常`~/.local/bin`で、rootで実行した場合は`/usr/local/bin`、`SSHC_INSTALL_DIR`を指定した場合はそのディレクトリです。
   - `make install`: `make uninstall`で`~/.local/bin/sshc`を削除します。
   - Windows: 下記の「インストール済みアプリ」または`uninstall.ps1`で削除します。停止とサービスの削除も自動で行います。
   - Android: ほかのアプリと同じ手順でアンインストールします。
3. 接続ごとのVPNを使った場合は、sshcが作成したコンテナイメージ`sshc-vpn`が残ります。VPNのコンテナはsshcエンジンの停止時に削除されます。イメージが不要なら、`docker image ls sshc-vpn`で確認してから`docker image rm`で削除してください。

Windowsでは、設定の「アプリ」→「インストール済みアプリ」からsshcを選んでアンインストールできます。PowerShellから削除する場合は次を実行します。`SSHC_INSTALL_DIR`で配置先を変えた場合は、`$dir`にそのディレクトリを指定してください。

```powershell
$dir = Join-Path $env:LOCALAPPDATA 'Programs\sshc'
powershell -NoProfile -ExecutionPolicy Bypass -File "$dir\uninstall.ps1"
```

削除用の`uninstall.ps1`は導入先に保存されるため、通信は不要です。sshcエンジンを停止し、sshcが作成したタスク、実行ファイル、導入記録、backup、ユーザーの`PATH`への追加、アプリ登録を削除します。途中で失敗した場合はそこで停止し、残ったファイルと未実行の手順を表示します。原因を解消してから`uninstall.ps1`を再実行してください。`%USERPROFILE%\.ssh\sshc`のデータと、`~/.ssh`の設定・鍵は残します。

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

---
title: インストール
description: macOS、Linux、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがsshcエンジン、CLI、Web UIを提供しています。

## macOS / Linux

[Homebrew](https://brew.sh/ja/)に対応しています。Homebrewを導入していない場合は、先に公式サイトの手順でHomebrewをインストールしてください。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。次は`v0.43.1`を導入する例です。

```sh
SSHC_VERSION=v0.43.1 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.43.1/install.sh | sh'
```

導入後は`sshc update`で更新できます。Homebrewで入れた場合はHomebrewから、`install.sh`で入れた場合は同じ配布元から更新されます。スクリプトなどで確認を省略する場合は、`sshc update --yes`を指定してください。

## Windows

Windows PowerShellからインストールできます。管理者権限は不要です。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

`%LOCALAPPDATA%\Programs\sshc`へインストールされ、ユーザーの`PATH`へ追加されます。更新にも同じコマンドを使えます。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases)から`sshc-android-v<version>.apk`をダウンロードできます。APKはReleaseワークフローで署名フィンガープリントとチェックサムを検査してから公開しています。

起動方法やファイル選択の動作は、[Android](/platform/android)で詳しく説明しています。

## 初回起動

```sh
sshc engine
```

対話ターミナルで起動した場合、初めて使うブラウザでは登録用の画面が自動で開きます。サービスとして起動したとき、画面が自動で開かなかったとき、別のブラウザを追加するときは、別のターミナルで次を実行してください。

```sh
sshc
```

`sshc engine`で起動するsshcエンジンは、フォアグラウンドプロセスです。常駐には、tmux、systemd、launchdなど、OSのプロセス管理機能を利用できます。

デスクトップ版は、最初に`http://127.0.0.1:54447/`で待ち受けます。既定のポートを別のプロセスが使用している場合は、空いているポートを1つ選び、このマシン固有のURLとして保存します。現在のURLは`sshc status`で確認できます。

Web UI自体は、この固定URLから開けます。初めて使うブラウザを登録するときだけ、`sshc`コマンドが一度限りの登録URLを開きます。一度登録したブラウザから同じポートへ接続した場合は、sshcエンジンを再起動した後も通常は再登録を求められません。保存したポートを使用できずURLが変わった場合は、以前のブラウザ登録が失効します。現在のsshcエンジンに対して、もう一度`sshc`を実行してください。別のブラウザやブラウザプロファイルを追加する場合も同じです。

ChromeやEdgeでは、ブラウザの「アプリをインストール」からsshcをWebアプリとして追加できます。Webアプリはsshcエンジンを起動するものではありません。先に`sshc engine`またはOSのサービスでsshcエンジンを動かしてください。

初回起動時にVaultのマスターパスワードを設定できます。

## Ubuntu / Linuxで常駐させる

systemdを使用しているLinuxでは、ユーザーサービスをインストールできます。sudoは不要です。

フォアグラウンドやtmuxで起動している`sshc engine`がある場合は、先にそのプロセスを停止してください。同じユーザーのsshcエンジンは同時に1つしか起動できないため、既存のsshcエンジンが動いているとsystemd側の起動に失敗します。

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

macOSでは、ユーザーエージェントとしてlaunchdへ登録できます。sudoは不要です。フォアグラウンドやtmuxで起動しているsshcエンジンを停止してから実行してください。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/Library/LaunchAgents/io.github.aida0710.sshc.plist`を作成し、現在のGUIユーザーへ登録してから、sshcエンジンが起動できたことを確認します。同じパスに手作業で作成したplistがある場合は上書きしません。削除は`sshc service disable`で行い、sshcが作成したplistだけを対象にします。

launchdがsshcエンジンを再起動するのは、sshcエンジンが異常終了した場合だけです。`sshc engine --replace`で置き換えた場合など、sshcエンジンが正常に終了した場合は再起動しません。サービスに戻すには`sshc service install`を再実行してください。

以前のバージョンのsshcで登録したplistは、`sshc service install`を再実行するまで、sshcエンジンが正常に終了しても再起動する以前の定義のままです。`sshc service status`、`sshc service restart`、`sshc update`は、この場合に`sshc service install`の再実行を案内します。

## 登録済みサービスを再起動する

LinuxとmacOSでは、稼働中のユーザーサービスを次のコマンドで再起動できます。

```sh
sshc service restart
```

サービス定義の場所と実行ファイルのパスを表示して、再起動するか確認します。再起動すると接続中のセッションと転送が終了します。自動化で確認を省略する場合は`sshc service restart --yes`を使用してください。

このコマンドで使えるのは、Homebrewまたはreceiptに対応した`install.sh`で導入され、この導入の安定パスと現在の定義が一致するサービスです。次の場合は再起動せず、終了コード1で終わります。

| 状態 | 対処 |
| --- | --- |
| 未登録・停止中 | `sshc service install`で登録・起動する |
| sshc管理外の定義 | 手書きの定義を確認し、そのサービスの管理方法で操作する |
| 古い定義 | `sshc service install`で現在の定義へ更新する |
| 別の実行ファイルを指す定義 | 登録した導入から実行する。現在の導入へ切り替える場合は`sshc service install`を使う |

確認待ちの間に定義や稼働状態が変わった場合も、成功扱いにはしません。`sshc service status`で状態を確認してから再実行してください。再起動後はサービスのPIDとsshcエンジンの起動情報、status APIを照合します。パスワードを設定したVaultはロックされるため、`sshc vault unlock`を実行してください。パスワードなしのVaultは自動でロックを解除します。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: インストール用のPowerShellコマンドを再実行
- Android: GitHub Releasesから新しいAPKをインストール

`sshc service install`で管理しているサービスが動作中で、サービス定義に記録された実行ファイルが今回の更新対象と一致する場合だけ、`sshc update`が更新後にサービスを再起動します。パスワードを設定したVaultは再起動でロックされるため、`sshc vault unlock`を実行してください。パスワードなしのVaultは、sshcエンジンの起動時に自動でロックを解除します。更新は成功したものの再起動だけに失敗した場合は、表示に従って`sshc service install`を再実行できます。`install.sh`を直接実行して更新した場合は、サービスを自動では再起動しないため、稼働中なら`sshc service restart`を実行してください。停止中や古い定義の場合は`sshc service install`を使います。サービス管理外のsshcエンジンは`sshc engine --replace`で再起動します。

## アンインストール

常駐させているsshcエンジンを先に止め、サービスの登録を削除してから、実行ファイルを削除します。サービスに登録したまま実行ファイルだけを削除すると、macOSのlaunchdは5秒ごとにsshcエンジンの起動を試みて失敗し続けます。Linuxのsystemdでも、ログインのたびに起動に失敗します。

1. `sshc service install`で登録している場合は、`sshc service disable`を実行します。sshcが作成したサービス定義だけを停止、無効化、削除します。フォアグラウンドやtmuxで起動したsshcエンジンは、そのプロセスを停止します。
2. 導入方法に合わせて実行ファイルを削除します。
   - Homebrew: `brew uninstall aida0710/tap/sshc`
   - `install.sh`: 配置先の`sshc`と、同じディレクトリにあるreceipt（`.sshc-install-receipt.json`）を削除します。配置先は通常`~/.local/bin`で、rootで実行した場合は`/usr/local/bin`、`SSHC_INSTALL_DIR`を指定した場合はそのディレクトリです。
   - `make install`: `make uninstall`で`~/.local/bin/sshc`を削除します。
   - Windows: 動いているsshcエンジンを停止してから、下のコマンドで`sshc.exe`を削除し、ユーザー`PATH`から配置先を外します。
   - Android: ほかのアプリと同じ手順でアンインストールします。
3. 接続ごとのVPNを使った場合は、sshcが作成したコンテナイメージ`sshc-vpn`が残ります。VPNのコンテナはsshcエンジンの停止時に削除されます。イメージが不要なら、`docker image ls sshc-vpn`で確認してから`docker image rm`で削除してください。

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

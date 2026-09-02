---
title: インストール
description: macOS、Linux、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがエンジン、CLI、Web UIを提供しています。

## macOS / Linux

[Homebrew](https://brew.sh/ja/)に対応しています。Homebrewを導入していない場合は、先に公式サイトの手順でHomebrewをインストールしてください。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。次は`v0.27.0`を導入する例です。

```sh
SSHC_VERSION=v0.27.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.27.0/install.sh | sh'
```

導入後は`sshc update`で更新できます。Homebrewで入れた場合はHomebrewから、`install.sh`で入れた場合は同じ配布元から更新されます。変更内容を事前に確認済みの自動処理では、`sshc update --yes`を指定して対話確認を省略できます。

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

対話ターミナルで起動した場合、初めて使うブラウザーでは登録用の画面が自動で開きます。サービスとして起動した場合、画面が開かない場合、別のブラウザーを追加する場合は、別のターミナルからUIを開きます。

```sh
sshc
```

`sshc engine`で起動するエンジンは、フォアグラウンドプロセスです。常駐には、tmux、systemd、launchdなど、OSのプロセス管理機能を利用できます。

デスクトップ版は、最初に`http://127.0.0.1:54447/`で待ち受けます。既定のポートを別のプロセスが使用している場合は、空いているポートを1つ選び、端末固有のURLとして保存します。現在のURLは`sshc status`で確認できます。

Web UI自体は、この固定URLから開けます。初めて使うブラウザーを登録するときだけ、`sshc`コマンドが一度限りの登録URLを開きます。一度登録したブラウザーから同じポートへ接続した場合は、エンジンを再起動した後も通常は再登録を求められません。保存したポートを使用できずURLが変わった場合は、以前のブラウザー登録が失効します。現在のエンジンに対して、もう一度`sshc`を実行してください。別のブラウザーやブラウザープロファイルを追加する場合も同じです。

ChromeやEdgeでは、ブラウザーの「アプリをインストール」からsshcをWebアプリとして追加できます。Webアプリはエンジンを起動するものではありません。先に`sshc engine`またはOSのサービスでエンジンを動かしてください。

初回起動時にVaultのマスターパスワードを設定できます。

## Ubuntu / Linuxで常駐させる

systemdを使用しているLinuxでは、ユーザーサービスをインストールできます。sudoは不要です。

フォアグラウンドやtmuxで起動している`sshc engine`がある場合は、先にそのプロセスを停止してください。同じユーザーのエンジンは同時に1つしか起動できないため、既存のエンジンが動いているとsystemd側の起動に失敗します。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/.config/systemd/user/sshc.service`を作成し、有効化してから、エンジンが起動できたことを確認します。Homebrew版では更新後も変わらない実行ファイルパスを、`install.sh`版では検出したインストール先をサービス定義へ登録します。手動配置やソースビルドは自動登録しません。

作成するsystemdユーザーサービスの内容と、登録する実行ファイルのパスを表示して確認を求めます。自動化で確認を省略するときは`sshc service install --yes`を使用してください。

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

macOSでは、ユーザーエージェントとしてlaunchdへ登録できます。sudoは不要です。フォアグラウンドやtmuxで起動しているエンジンを停止してから実行してください。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/Library/LaunchAgents/io.github.aida0710.sshc.plist`を作成し、現在のGUIユーザーへ登録してから、エンジンが起動できたことを確認します。同じパスに手作業で作成したplistがある場合は上書きしません。削除は`sshc service disable`で行い、sshcが作成したplistだけを対象にします。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: インストール用のPowerShellコマンドを再実行
- Android: GitHub Releasesから新しいAPKをインストール

`sshc service install`で管理しているサービスが動作中で、サービス定義に記録された実行ファイルが今回の更新対象と一致する場合だけ、`sshc update`が更新後に再起動します。再起動後はVaultがロックされるため、`sshc vault unlock`を実行してください。更新は成功したものの再起動だけに失敗した場合は、表示に従って`sshc service install`を再実行できます。サービス管理外のエンジンは`sshc engine --replace`で再起動します。

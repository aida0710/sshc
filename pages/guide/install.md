---
title: インストール
description: macOS、Linux、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがエンジン、CLI、Web UIを提供しています。

## macOS / Linux

[Homebrew](https://brew.sh/ja/)に対応しています。Homebrewが未導入の場合は、公式サイトの手順でインストールできます。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。次は`v0.23.0`を導入する例です。

```sh
SSHC_VERSION=v0.23.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.23.0/install.sh | sh'
```

導入後は`sshc update`で更新できます。Homebrewで入れた場合はHomebrewから、`install.sh`で入れた場合は同じ配布元から更新されます。

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

デスクトップ版は`http://127.0.0.1:54447/`を最初に使用します。別のユーザーやアプリが使用中なら空きポートを一度だけ選び、端末固有のURLとして保存します。現在のURLは`sshc status`で確認できます。`sshc`が開く一度だけ有効なURLでブラウザーを最初に登録すると、以後はブックマークやインストールしたWebアプリから直接開けます。engineを再起動した場合も、同じブラウザーなら自動で再認証します。別のブラウザーやブラウザープロファイルでは、もう一度`sshc`を実行して登録してください。

ChromeやEdgeでは、ブラウザーの「アプリをインストール」からsshcをWebアプリとして追加できます。Webアプリはengineを起動するものではありません。先に`sshc engine`またはOSのサービスでengineを動かしてください。

初回起動時にVaultのマスターパスワードを設定できます。

## Ubuntu / Linuxで常駐させる

systemdを使用しているLinuxでは、ユーザーサービスをインストールできます。sudoは不要です。

フォアグラウンドやtmuxで起動している`sshc engine`がある場合は、先にそのプロセスを停止してください。既存engineがlockを保持したままでは、systemd側のengineを起動できません。

```sh
sshc service install
sshc service status
sshc vault unlock
```

`service install`は`~/.config/systemd/user/sshc.service`を作成し、有効化して起動します。systemdのMain PIDとsshcのstatus APIを確認してから成功を返します。Homebrew版では更新後も変わらない`opt/sshc/bin/sshc`を、`install.sh`版では確認済みのインストール先を登録します。手動配置やソースビルドは自動登録しません。

同名のunitがすでに手作業で作成されている場合、sshcは上書きしません。既存unitを停止して退避してから実行してください。

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

サービスを削除する場合は`sshc service disable`を実行します。このコマンドもsshcが作成したunitだけを削除します。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: インストール用のPowerShellコマンドを再実行
- Android: GitHub Releasesから新しいAPKをインストール

`sshc service install`で管理しているサービスが動作中で、登録先が今回の更新対象と一致する場合だけ、`sshc update`が更新後に再起動します。再起動後はVaultがロックされるため、`sshc vault unlock`を実行してください。更新は成功したものの再起動だけに失敗した場合は、表示に従って`sshc service install`を再実行できます。サービス管理外のエンジンは`sshc engine --replace`で再起動します。

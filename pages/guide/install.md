---
title: インストール
description: macOS、Linux、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがエンジン、CLI、Web UIを提供しています。

## macOS / Linux

Homebrewに対応しています。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、インストーラーとバイナリのバージョンを同じReleaseタグに固定してください。次は`v0.21.1`を導入する例です。

```sh
SSHC_VERSION=v0.21.1 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.21.1/install.sh | sh'
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

別のターミナルからUIを開けます。

```sh
sshc
```

`sshc engine`で起動するエンジンは、フォアグラウンドプロセスです。常駐には、tmux、systemd、launchdなど、OSのプロセス管理機能を利用できます。

起動後、別のターミナルから`sshc`を実行すると、一度だけ有効なローカルUIのURLがブラウザーで開きます。初回起動時にVaultのマスターパスワードを設定できます。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: インストール用のPowerShellコマンドを再実行
- Android: GitHub Releasesから新しいAPKをインストール

更新後にCLIとエンジンのバージョンが異なる場合は、`sshc engine --replace`でエンジンを再起動してください。

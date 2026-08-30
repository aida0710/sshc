---
title: インストール
description: macOS、Linux、Windows、Androidにsshcをインストールする。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できるターミナルアプリです。デスクトップ版では、1つの`sshc`バイナリがエンジン、CLI、Web UIを提供します。

## macOS / Linux

もっとも簡単な方法はHomebrewです。

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

Windows PowerShellから実行します。管理者権限は不要です。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

`%LOCALAPPDATA%\Programs\sshc`へインストールし、ユーザーの`PATH`へ追加します。更新するときも、同じコマンドを実行してください。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases)から`sshc-android-v<version>.apk`をダウンロードします。APKはReleaseワークフローで署名フィンガープリントとチェックサムを検査してから公開しています。

起動方法やファイル選択の動作は、[Android](/platform/android)で詳しく説明しています。

## 初回起動

```sh
sshc engine
```

別のターミナルでUIを開きます。

```sh
sshc
```

`sshc engine`はフォアグラウンドで動作します。常駐させる場合は、tmux、systemd、launchdなど、OSのプロセス管理機能を使用してください。

起動後、別のターミナルから`sshc`を実行すると、一度だけ有効なローカルUIのURLがブラウザーで開きます。初回はVaultのマスターパスワードを設定してください。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: インストール用のPowerShellコマンドを再実行
- Android: GitHub Releasesから新しいAPKをインストール

更新後にCLIとエンジンのバージョンが異なる場合は、`sshc engine --replace`でエンジンを再起動してください。

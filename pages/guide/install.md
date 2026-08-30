---
title: インストール
description: macOS、Linux、Windows、Androidへsshcを導入する。
---

# インストール

sshcはmacOS、Linux、Windows、Androidで利用できます。Desktop版は一つの`sshc` binaryがengine、CLI、Web UIを提供します。

## macOS / Linux

Homebrewを使う方法が最短です。

```sh
brew install aida0710/tap/sshc
```

Homebrewを使わない場合は、取得するinstallerとbinaryを同じRelease tagへ固定します。次は`v0.21.0`を導入する例です。

```sh
SSHC_VERSION=v0.21.0 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.21.0/install.sh | sh'
```

導入後は`sshc update`で、Homebrewまたはreceipt対応版installerの管理元へ更新を委ねられます。

## Windows

Windows PowerShellから実行します。管理者権限は不要です。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

`%LOCALAPPDATA%\Programs\sshc`へ配置し、ユーザー`PATH`へ追加します。同じコマンドを再実行すると最新の安定版へ更新します。

## Android

[GitHub Releases](https://github.com/aida0710/sshc/releases)から`sshc-android-v<version>.apk`を取得します。APKはRelease workflowが署名fingerprintとchecksumを検査してから公開します。

詳しい起動方法とfile pickerの動作は[Android](/platform/android)を参照してください。

## 初回起動

```sh
sshc engine
```

別のターミナルでUIを開きます。

```sh
sshc
```

`sshc engine`はforegroundで動きます。常駐させる場合はtmux、systemd、launchdなど、OSのprocess管理機能を使用してください。

起動後、別のterminalから`sshc`を実行すると一度だけ使えるlocal UI URLを開きます。初回はVaultのmaster passwordを設定します。

## 更新

- Homebrew／`install.sh`: `sshc update`
- Windows: install用PowerShell commandを再実行
- Android: GitHub Releaseから新しいAPKをinstall

更新後にCLIとengineのversionが異なる場合は`sshc engine --replace`でengineを再起動してください。

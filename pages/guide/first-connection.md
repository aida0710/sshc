---
title: 最初の接続
description: Vaultを作成し、既存または新しいSSH接続を開く。
---

# 最初の接続

## 1. Vaultを作成する

初回起動ではmaster passwordを2回入力します。Vaultは保存済みaccount password、鍵passphrase、Snippetのsecret、同期credentialを暗号化します。

::: warning
master passwordは復旧できません。同期用の暗号化keyとは別物で、端末ごとに異なる値を使用できます。
:::

## 2. 接続先を確認する

既存の`~/.ssh/config`があれば、具体的な`Host` aliasがConnectionsへ表示されます。HomeのQuick accessでは名前、group、tag、`user@host:port`で検索できます。

新規作成は**Connections → New connection**から行います。まずalias、host、user、portを入力し、必要なら鍵、password、ProxyJumpを設定します。

## 3. 接続前に検査する

- **到達性を確認**: DNS／TCP／host keyまでを確認
- **保存済み設定で認証を確認**: 保存した鍵やpasswordで認証まで確認
- **設定解析**: `Include`や`Match`を含む最終的な値と出典を確認

未知のhost keyはfingerprintを確認してから登録します。保存済みhost keyが変化した場合、sshcは接続を拒否します。

## 4. 開く

Connectionsの**接続**、Homeのpanelをtap、またはdesktopでpanelをdouble clickするとTerminalを開きます。CLIなら次の通りです。

```sh
sshc ssh <alias>
```

接続中は名前解決、踏み台、host key、認証、shell開始の段階が表示されます。失敗した場合は画面のerror codeと原因を確認してください。

---
title: 認証情報とVault
description: Account password、鍵passphrase、master passwordの役割と編集方法。
---

# 認証情報とVault

## 3種類のsecret

| secret | 用途 | 共有 |
| --- | --- | --- |
| Master password | この端末のVaultを開く | 端末ごとでよい |
| Account password | SSH serverのpassword認証 | 接続先に紐付く |
| Key passphrase | 秘密鍵の復号 | 鍵に紐付く |

同期を使う場合は、これとは別にsync keyがあります。

## Account passwordを保存する

Secretsでlabelとpasswordを作成し、接続先へ割り当てます。編集時は保存済み値をVaultから復号してinputへ戻せます。画面を離れる、Vaultをlockする、保存を完了すると平文stateを破棄します。

## 鍵passphrase

鍵画面から秘密鍵ごとに保存、編集、削除できます。ProxyJumpでは最終接続先だけでなく各hopの鍵passphrase／account passwordを別々に解決します。

## Lockとpassword変更

```sh
sshc vault status
sshc vault lock
sshc vault change-password
```

Vaultをlockしても既に開いているSSH sessionは閉じませんが、新しいsecret操作はできなくなります。12時間操作がない場合は自動lockします。

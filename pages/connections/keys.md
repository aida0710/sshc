---
title: 鍵とKnown Hosts
description: SSH鍵の生成、import、編集、agent、remote登録、host key管理。
---

# 鍵とKnown Hosts

## 鍵

鍵画面ではprivate／public key、fingerprint、algorithm、配置先を確認できます。生成、import、名前変更、passphrase編集、公開鍵copy、SSH agentへの追加に対応します。

sshcが管理する鍵はgroup構造に合わせて`~/.ssh/keys/...`へ置かれることがあります。接続設定では実際に存在する`IdentityFile`を使用してください。同期対象には`~/.ssh`配下の秘密鍵も含まれ、端末上で暗号化されてから送信されます。

## Serverへ公開鍵を登録

複数のremote keyを検索・選択し、対象serverへ登録できます。接続先の設定を解決してから実行するため、その接続が使用しない別aliasの`ProxyCommand` warningを混ぜません。

## Known Hosts

known hosts画面はhost、algorithm、fingerprintなどの列でsortできます。未知のkeyは接続時に確認し、変更されたkeyは自動上書きしません。

::: warning
fingerprintの変更がserver再構築によるものだと確認できるまで、古いentryを削除しないでください。
:::

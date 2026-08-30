---
title: トラブルシューティング
description: engine、vault、鍵、同期、SSH接続で問題が起きたときの確認項目。
---

# トラブルシューティング

## Engineを起動できない

```sh
sshc status
sshc engine --replace
```

CLIとengineのversionが違う場合は、古いengineを停止し、更新後のbinaryから起動し直します。Androidではエラー画面の「診断情報を表示」からversion、error code、operation、OS情報を確認できます。秘密は診断reportへ含めません。

## Vaultを開けない

master passwordは復旧できません。初回作成直後から開けない場合は、表示されたerror codeと「原因」の詳細を確認してください。`vault_too_new`は現在のbinaryが対応する設定versionよりvaultが新しい状態です。新しいsshcへ更新してから再試行します。

## 同期後に秘密鍵が見つからない

Connectionの`IdentityFile`が実際の同期先pathを指しているか確認します。sshcで管理した鍵はgroup構造に合わせて`~/.ssh/keys/...`へ置かれることがあります。絶対pathや古い直下pathを設定に残さず、接続詳細に表示される解決済みpathを確認してください。

## ProxyJumpでpasswordを求められる

最終接続先だけでなく、各jump hostに対応するpasswordまたはkey passphraseが保存されている必要があります。Connectionの認証testで、どのhopが失敗したかを確認します。sshcは2FAなど保存値で答えられないpromptをTerminalへ表示します。

## Syncが進まない

```sh
sshc sync
sshc sync now
```

vaultの解錠、sync方向、bucket state、last checkを確認します。`outcome_unknown`は変更送信後の結果を確定できない状態です。同じmutationをすぐ再実行せず、remote状態を更新してから判断してください。

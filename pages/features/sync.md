---
title: 暗号化同期
description: S3互換storageを使ったsshcの暗号化snapshot同期。
---

# 暗号化同期

![同期の状態と変更確認画面](/images/sync-desktop.png)

sshcはworkspaceを端末上で暗号化してから、S3互換object storageへsnapshotとして保存します。storage事業者へ平文は送りません。

## 最初の端末

1. bucket、endpoint、access keyを入力する
2. 接続確認を通す
3. 空の保存先ならsync keyを生成する
4. 一度だけ表示されるkeyを安全な場所へ保管する

## 2台目以降

同じbucket pathとsync keyを入力します。remoteに既存snapshotがある場合、正しいkeyで復号できてから設定を保存します。別のdatasetを作る場合はbucket内のpathを変えます。

同期方向は送受信、送信専用、受信専用から選べます。2台目を閲覧用にする場合は受信専用を選びます。

## Git風の操作

- **変更を確認**: localとremoteの差分をpreview
- **push**: 確認したremote revisionへ条件付き送信
- **pull**: conflictやremovalがなければ適用
- **force**: previewしたexact ETag／revisionだけを対象に適用
- **履歴**: bucket内の過去snapshotを取得して表示

自動同期はremoteの更新を定期確認します。sshcでlocal内容を変更したときだけ必要なpushを行い、変化のないsnapshotを毎回uploadしません。

::: warning Sync key
master passwordは端末ごとに異なって構いません。sync keyは同じ保存先を使う端末で共通です。紛失するとremote snapshotを復号できません。
:::

詳しい競合処理と履歴は[Push・Pull・履歴](/sync/workflow)を参照してください。

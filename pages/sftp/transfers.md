---
title: Transfer Manager
description: 大量のfileとfolderをqueueで転送し、pause、resume、retry、cancelする。
---

# Transfer Manager

![file転送中のTransfer Manager](/images/transfer-manager.png)

file upload、folder upload、file download、folder downloadを共通queueで管理します。既定の同時実行数は2件です。

## 表示される情報

- 全体とfile単位の進捗
- 転送済みbyte／総byte
- 現在速度と残り時間
- queued、running、paused、completed、failed、canceledの状態

## 操作

- **Pause**: 新しいread／writeを止め、再開情報を保持
- **Resume**: remoteまたはlocalの転送済みsizeから続行
- **Retry**: 失敗したfileだけ再実行
- **Cancel**: jobを中止してqueueから終了扱いにする
- **Clear finished**: 完了済みjobだけ表示から除く

uploadは同じdirectoryの一時fileへ転送し、完了後にatomic renameします。既存fileへ上書きする場合は開始前に確認します。folder downloadはZIPとしてstreamし、Androidではsystem file pickerへ渡します。

## 画面を離れた場合

transferはWeb pageではなくengineが所有するため、SFTPから別pageへ移動しても続きます。engineを終了するとrunning jobは止まります。次回の自動復元ではなく、明示的なresume／retryを使用します。

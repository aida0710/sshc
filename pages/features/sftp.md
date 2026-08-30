---
title: SFTP
description: remote file操作と中断再開に対応したSFTP Transfer Manager。
---

# SFTP

![SFTPのfile browser](/images/sftp-desktop.png)

接続先を選ぶまでSFTP接続を開始せず、保存済みのSSH設定、host key、資格情報を利用します。

## File操作

- remote directoryの移動、作成、rename、chmod、delete
- file／folderの選択とDrag & Drop upload
- file download、folderのZIP download
- 2 MiB以下のUTF-8 textをMonaco Editorで編集

## Transfer Manager

fileとfolderを一つのqueueで扱い、既定では同時2件まで転送します。file単位の進捗、速度、残り時間を表示し、pause、resume、retry、cancel、失敗fileだけの再実行が可能です。

大きなuploadはremoteの一時fileへ送り、完了時にatomic renameします。接続が切れた場合はremote側の転送済みsizeから再開します。downloadはHTTP Rangeで再開し、SFTP画面を離れてもqueueはengine側で継続します。

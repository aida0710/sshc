---
title: 機能
description: sshcがOpenSSHへ追加する接続、Terminal、SFTP、Workspace、同期機能。
---

# 機能

sshcはOpenSSH設定を正本のまま扱い、その周囲に操作画面とautomationを追加します。

## 接続管理

- `~/.ssh/config`、`Include`、`Match`を解析
- コメント、記述順、空白を保った編集
- host、設定file、Snippet、設定を`Ctrl/Cmd+K`から横断検索
- ProxyJump、鍵、保存済み資格情報を同じ接続経路で利用し、ProxyCommandは解析結果として表示

## 日常の操作

- [Terminal](./terminal): 検索、再接続、Quick Commands、文字コード、port forwarding
- [SFTP](./sftp): folder転送、resume、background queue、Monaco Editor
- [Workspace](./workspace): 最大4 pane、Drag & Drop、Focus Mode、一括入力
- [暗号化同期](./sync): S3互換storageへの条件付きpush／pullと履歴

## CLI

Web UIとCLIは同じengineと設定解決器を利用します。対話SSH、非対話command、状態確認、同期、Serial、Telnetをshellから操作できます。

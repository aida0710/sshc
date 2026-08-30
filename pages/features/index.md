---
title: 機能
description: OpenSSH設定の整理、認証情報の再利用、AIエージェント向けCLI、暗号化同期。
---

# 機能

sshcはOpenSSH設定を正本として使いながら、認証情報の再利用、CLIによる自動化、暗号化同期に必要な機能を加えます。

## 接続管理

- `~/.ssh/config`、`Include`、`Match`を解析
- コメント、記述順、空白をできるだけ崩さずに編集
- ホスト、設定ファイル、スニペット、設定を`Ctrl/Cmd+K`で横断検索
- 解決した接続経路に沿ってProxyJump、鍵、保存済み資格情報を使用。ProxyCommandは解析結果として表示
- OpenSSH形式を保つため、通常の`ssh`、VS Code、Codexからも同じエイリアスを利用可能

## 認証情報

パスワードと鍵のパスフレーズはVaultで暗号化し、接続先や鍵に割り当てます。一度保存すれば、Terminal、SFTP、ProxyJump、CLIが同じ認証情報を必要に応じて利用します。

## 日常の操作

- [Terminal](./terminal): 検索、再接続、クイックコマンド、文字コード、ポート転送
- [SFTP](./sftp): フォルダー転送、中断再開、バックグラウンド転送、Monaco Editor
- [Workspace](./workspace): 最大4ペイン、ドラッグ＆ドロップ、フォーカス表示、一括入力
- [暗号化同期](./sync): S3互換ストレージへの条件付きPush／Pullと履歴

## CLI

Web UIとCLIは、同じエンジン、OpenSSH設定、Vaultを使います。CodexなどのAIエージェントは`sshc ssh <alias> --non-interactive -- <command...>`を直接実行でき、Vaultが開いていればsshcが保存済みのパスワードや鍵のパスフレーズを認証に使います。

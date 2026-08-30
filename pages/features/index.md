---
title: 機能
description: SSHとローカルシェル、SFTP、OpenSSH接続管理、認証情報、AIエージェント向けCLI、暗号化同期。
---

# 機能

sshcは、SSHとローカルシェルをアプリ内のTerminalで操作するローカルアプリケーションです。SFTP、ポート転送、複数ペインに加え、OpenSSH接続管理、認証情報の再利用、CLI、暗号化同期を備えています。

## Terminal

- SSHとローカルシェルを同じ画面で操作
- 終了したSSHセッションへ、ペインとスクロールバックを残したまま再接続
- 検索、クイックコマンド、文字コード、Local forwarding、Dynamic SOCKS
- [Workspace](./workspace)で最大4ペインを配置し、レイアウトと一括入力を管理

## 接続管理

- `~/.ssh/config`、`Include`、`Match`を解析
- コメント、記述順、空白をできるだけ崩さずに編集
- ホスト、設定ファイル、スニペット、設定を`Ctrl/Cmd+K`で横断検索
- 解決した接続経路に沿ってProxyJump、鍵、保存済み資格情報を使用。ProxyCommandは解析結果として表示
- OpenSSH形式を保つため、通常の`ssh`、VS Code、Codexからも同じエイリアスを利用可能

## 認証情報

パスワードと鍵のパスフレーズはVaultで暗号化し、接続先や鍵に割り当てます。一度保存すれば、Terminal、SFTP、ProxyJump、CLIが同じ認証情報を必要に応じて利用します。

## SFTP

[SFTP](./sftp)では、リモートファイルの閲覧と編集、フォルダー転送、中断と再開、バックグラウンド転送を利用できます。

## CLI

Web UIとCLIは、同じエンジン、OpenSSH設定、Vaultを使います。CodexなどのAIエージェントは`sshc ssh <alias> --non-interactive -- <command...>`を直接実行でき、Vaultが開いていればsshcが保存済みのパスワードや鍵のパスフレーズを認証に使います。

## 暗号化同期

[暗号化同期](./sync)は、接続設定、鍵、資格情報、スニペットを端末上で暗号化し、S3互換ストレージへPush／Pullします。同期先へ平文は送信しません。

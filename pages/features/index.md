---
title: 機能
description: SSHとローカルシェル、SFTP、OpenSSH接続管理、認証情報、接続ごとのVPN、AIエージェント向けCLI、暗号化同期。
---

# 機能

sshcは、SSHとローカルシェルを扱うターミナルアプリです。SFTP、ポート転送、複数ペインに加え、OpenSSH接続管理、認証情報の再利用、接続ごとのVPN、CLI、暗号化同期を備えています。

## Terminal

- SSHとローカルシェルを同じ画面で操作
- 終了したSSHセッションへ、ペインとスクロールバックを残したまま再接続
- 検索、クイックコマンド、文字コード、Local forwarding、Dynamic SOCKS
- [Workspace](./workspace)で最大4ペインを配置し、レイアウトと一括入力を管理

## 接続管理

- `~/.ssh/config`、`Include`、`Match`を解析
- コメント、記述順、空白をできるだけ崩さずに編集
- ホスト、設定ファイル、スニペット、設定を`Ctrl/Cmd+K`で横断検索
- 接続設定に従ってProxyJump、鍵、保存済みの認証情報を使用
- `ProxyCommand`をローカルで実行し、その標準入出力をSSH接続に使用
- OpenSSH形式を保ち、通常の`ssh`、VS Code、Codexでも同じエイリアスを使用

## 認証情報

パスワード、鍵のパスフレーズ、TOTPのセットアップキーはVaultに暗号化して保存し、接続先や鍵に割り当てます。Vaultへ一度登録すれば、Terminal、SFTP、ProxyJump、CLIから再利用できます。サーバーがワンタイムパスワードの入力を明示的に求めたときは、接続先に割り当てたTOTPのコードを自動入力します。

## VPN

[接続ごとのVPN](./vpn)では、選んだSSH接続だけを、その接続専用のVPN経由で接続します。このマシンのルーティングとDNSは変更しません。WireGuard、L2TP/IPsec、OpenConnect、OpenVPN、IKEv2/IPsecに対応し、Dockerが動いているマシンで使用できます。VPNのシークレットはVaultに保存します。

## SFTP

[SFTP](./sftp)では、リモートファイルを閲覧・編集できます。デスクトップでは2つの接続先を並べ、ディレクトリの差分確認とRemote→Remoteのコピー／移動を行えます。転送キューはsshcエンジンが管理・保存するため、別画面への移動やsshcエンジン再起動後も状態を復元できます。

## CLI

Web UIとCLIは、共通のsshcエンジン、OpenSSH設定、Vaultを使っています。CodexなどのAIエージェントは`sshc ssh <alias> --non-interactive -- <command...>`を直接実行できます。Vaultのロックが解除されていれば、sshcが保存済みのパスワードや鍵のパスフレーズを使って認証します。

## 暗号化同期

[暗号化同期](./sync)では、接続設定、認証情報、スニペット、同期対象から除外していないSSH鍵をこのマシン上で暗号化し、利用者が用意したS3互換ストレージへPush／Pullできます。ストレージ事業者は、これらの内容を平文では取得できません。バケット名やS3オブジェクト名など、暗号化されない情報は[セキュリティ](/reference/security)で確認できます。

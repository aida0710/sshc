---
title: ポート転送
description: 接続設定に保存した、または一時的に使うLocal forwardingとDynamic SOCKSを管理する。
---

# ポート転送

![Local forwardingの設定](/images/port-forwarding.png)

sshcはLocal forwardingとDynamic SOCKS5に対応しています。Remote forwardingには対応していません。

## Local forwarding

ローカル側のアドレス／ポートで待ち受け、SSH接続先から転送先のホスト／ポートへ接続します。

```text
127.0.0.1:8080  →  SSHホスト  →  127.0.0.1:80
```

転送先には、SSHサーバーから見える任意のホストを指定できます。同じサーバー上のサービスへつなぐ場合は、`127.0.0.1`が分かりやすい例です。

## Dynamic SOCKS

ローカル側にSOCKS5エンドポイントを開き、アプリケーションが通信ごとに転送先を指定します。そのため、Dynamicの設定には固定の転送先入力欄がありません。

```text
SOCKSクライアント  →  127.0.0.1:1080  →  SSHホスト  →  通信先
```

## 保存する転送と一時的な転送

Connectionsのsshcタブでは、接続先ごとに転送設定を保存できます。接続中のTerminalからは、現在のSSH接続を使って一時的な転送を開始、停止できます。

待受先はループバックアドレスに限られます。ただし、同じ端末上の別プロセスや別ユーザーを強く隔離するセキュリティ境界ではありません。転送先のサービスにも、適切な認証を設定してください。

---
title: 踏み台接続
description: ProxyJumpを使う多段SSH接続と認証の考え方。
---

# 踏み台接続

sshcは`ProxyJump`をprocess内で辿り、最終接続先まで同じhost key／credential policyを適用します。

```ssh-config
Host bastion
  HostName 203.0.113.10
  User ops

Host internal
  HostName 10.0.0.20
  User deploy
  ProxyJump bastion
```

## 認証

踏み台と最終hostは別の認証先です。それぞれのaliasへ鍵、key passphrase、account passwordを保存してください。保存値で回答できない2FAなどのpromptはinteractive Terminalへ表示します。

接続中は「踏み台へ接続」「踏み台のhost key」「踏み台の認証」「接続先の認証」のように段階を区別します。失敗時はどのhopで止まったかを診断に含めます。

## ProxyCommand

設定解析では`ProxyCommand`を表示できますが、sshcのprocess内SSH clientは任意の外部commandを実行しません。sshcで接続するaliasには`ProxyJump`を使用してください。

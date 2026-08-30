---
title: 踏み台接続
description: ProxyJumpを使う多段SSH接続と認証の考え方。
---

# 踏み台接続

sshcは`ProxyJump`を内部でたどり、踏み台から最終接続先まで、同じホスト鍵と資格情報の方針を適用します。

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

踏み台と最終接続先は、別々の認証先です。それぞれのエイリアスに、必要な鍵、鍵のパスフレーズ、アカウントパスワードを保存してください。保存済みの値では答えられない2FAなどの確認は、対話Terminalに表示されます。

接続中は「踏み台へ接続」「踏み台のホスト鍵」「踏み台の認証」「接続先の認証」のように、段階を分けて表示します。失敗した場合は、どの接続先で止まったのかを診断情報で確認できます。

## ProxyCommand

設定解析では`ProxyCommand`の内容を確認できます。ただし、sshc内蔵のSSHクライアントは任意の外部コマンドを実行しません。sshcから接続するエイリアスには、`ProxyJump`を使用してください。

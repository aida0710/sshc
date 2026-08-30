---
title: 踏み台接続
description: ProxyJumpを使う多段SSH接続と認証の考え方。
---

# 踏み台接続

sshcは`ProxyJump`を内部でたどり、踏み台から最終接続先まで、一貫した方法でホスト鍵と認証情報を確認しています。

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

踏み台と最終接続先は、別々の認証先です。それぞれのエイリアスに、必要な鍵、鍵のパスフレーズ、アカウントパスワードを保存してください。2FAなどの追加入力は、対話Terminalで行えます。

接続中は「踏み台へ接続」「踏み台のホスト鍵」「踏み台の認証」「接続先の認証」のように、段階を分けて表示しています。失敗した場合は、どの接続先で止まったのかを診断情報で確認できます。

## ProxyCommand

`ProxyCommand`を設定した接続先では、sshcがコマンドをこの端末上で実行し、その標準入出力をSSH接続に使います。Unix系OSでは`/bin/sh`、Windowsではコマンドインタープリターを介して起動します。接続ログには、実行するコマンドが表示されます。

同じ接続先に`ProxyJump`と`ProxyCommand`を同時に指定した設定は拒否されます。また、すでに別の踏み台へ接続した後のホップに`ProxyCommand`がある場合も実行されません。コマンドは踏み台ではなく、この端末上で動くためです。

`ProxyCommand`はローカルコマンドを実行できる設定です。内容を確認し、信頼できるSSH設定だけを使用してください。

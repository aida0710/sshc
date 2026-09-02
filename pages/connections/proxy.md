---
title: 踏み台接続
description: ProxyJumpを使う多段SSH接続と認証の考え方。
---

# 踏み台接続

sshcは`ProxyJump`に指定された踏み台へ順番に接続します。各踏み台と最終接続先でホスト鍵を検証し、それぞれに対応する認証情報を使います。

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

踏み台と最終接続先は、別々の認証先です。使用する秘密鍵はそれぞれの接続設定へ指定し、鍵のパスフレーズとアカウントパスワードはVaultで各接続先へ割り当ててください。2FAなどの追加認証は、対話Terminalで入力できます。

接続中は「踏み台へ接続」「踏み台のホスト鍵」「踏み台の認証」「接続先の認証」のように、段階を分けて表示しています。失敗した場合は、どの接続先で止まったのかを診断情報で確認できます。

## ProxyCommand

`ProxyCommand`を設定した接続先では、sshcを実行しているローカル端末上でコマンドを起動し、その標準入出力をSSH接続に使います。Unix系OSでは`/bin/sh`、Windowsでは`cmd.exe`を介して起動します。接続ログには、実行するコマンドが表示されます。

同じ接続先に`ProxyJump`と`ProxyCommand`を同時に指定した設定は拒否されます。また、すでに別の踏み台へ接続した後のホップに`ProxyCommand`がある場合も実行されません。コマンドは踏み台ではなく、sshcを実行しているローカル端末上で動くためです。

`ProxyCommand`はローカルコマンドを実行できる設定です。内容を確認し、信頼できるSSH設定だけを使用してください。

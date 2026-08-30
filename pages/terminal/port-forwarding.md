---
title: Port forwarding
description: 保存済みまたは一時的なLocal forwardingとDynamic SOCKSを管理する。
---

# Port forwarding

![Local forwardingの設定](/images/port-forwarding.png)

sshcはLocal forwardingとDynamic SOCKS5を提供します。Remote forwardingは提供しません。

## Local forwarding

local側のaddress／portで待ち受け、SSH接続先からdestination host／portへ接続します。

```text
127.0.0.1:8080  →  SSH host  →  127.0.0.1:80
```

destinationはSSH serverから見える任意のhostを指定できます。同じserver上のserviceなら`127.0.0.1`が分かりやすい既定例です。

## Dynamic SOCKS

local側にSOCKS5 endpointを開き、applicationごとにdestinationを指定します。そのためDynamic設定には固定destination fieldがありません。

```text
SOCKS client  →  127.0.0.1:1080  →  SSH host  →  requested destination
```

## 保存と一時forward

Connectionsのsshc tabでは接続先へ設定を保存できます。接続中のTerminalからは、そのSSH transportを再利用して一時forwardを開始、停止できます。

listenerはloopbackへ限定されます。ただし同じ端末上の別process／userを強く隔離するsecurity boundaryではありません。forward先のserviceにも適切な認証を設定してください。

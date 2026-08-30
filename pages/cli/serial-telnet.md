---
title: SerialとTelnet
description: Serial console、Telnet、文字コード、非対話automationを使う。
---

# SerialとTelnet

## Serial

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

data bits、parity、stop bits、flow control、DTR、RTS、break durationを指定できます。device名はOSごとに異なります。

## Telnet

```sh
sshc telnet 192.0.2.20:23 --encoding shift_jis
```

Telnetは平文で、server認証もありません。信頼できる隔離networkやVPNの内側など、別の保護がある場合に限定してください。

## 文字コード

SerialとTelnetは`utf-8`、`shift_jis`、`euc-jp`、`iso-2022-jp`に対応します。SSHの文字コードは接続先ごとのsshc設定へ保存します。

## 非対話automation

```sh
sshc serial /dev/ttyUSB0 --non-interactive \
  --expect 'login:' --timeout 20s -- 'admin'

sshc telnet 192.0.2.20:23 --non-interactive \
  --script ./steps.json --json -- ''
```

`--expect`または`--read-for`、timeout、settle time、最大byte、line ending、output必須条件を指定できます。automationは受信量を上限で切り、JSON modeでは結果とwarningを一つのobjectで返します。

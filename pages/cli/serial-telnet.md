---
title: SerialとTelnet
description: Serialコンソール、Telnet、文字コード、非対話の自動化を使う。
---

# SerialとTelnet

## Serial

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

データビット、パリティ、ストップビット、フロー制御、DTR、RTS、ブレーク時間を指定できます。デバイス名はOSによって異なります。

## Telnet

```sh
sshc telnet 192.0.2.20:23 --encoding shift_jis
```

Telnetの通信は平文で、サーバー認証もありません。信頼できる隔離ネットワークやVPNの内側など、別の保護がある環境だけで使用してください。

## 文字コード

SerialとTelnetは、`utf-8`、`shift_jis`、`euc-jp`、`iso-2022-jp`に対応しています。SSHの文字コードは、接続先ごとのsshc設定に保存します。

## 非対話の自動化

```sh
sshc serial /dev/ttyUSB0 --non-interactive \
  --expect 'login:' --timeout 20s -- 'admin'

sshc telnet 192.0.2.20:23 --non-interactive \
  --script ./steps.json --json -- ''
```

`--expect`または`--read-for`のほか、タイムアウト、待機時間、最大受信量、改行コード、必須の出力を指定できます。受信量は設定した上限で打ち切られ、JSONモードでは結果と警告が一つのオブジェクトとして返ります。

---
title: SerialとTelnet
description: SerialコンソールとTelnetを、対話操作または自動処理から使う。
---

# SerialとTelnet

## Serial

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

データビット、パリティ、ストップビット、DTR、RTSを接続時に指定できます。フロー制御は`none`、`rtscts`、`xonxoff`に対応しています。接続中は、DTRとRTSの切り替えや、指定した長さのブレーク送信も行えます。これらの操作はフロー制御の設定とは別です。

デバイス名はOSによって異なります。Linuxでは`/dev/ttyUSB0`、macOSでは`/dev/cu.*`、Windowsでは`COM3`などを指定します。

## Telnet

```sh
sshc telnet 192.0.2.20:23 --encoding shift_jis
```

Telnetの通信は平文で、サーバー認証もありません。信頼できる隔離ネットワークやVPNの内側など、別の保護がある環境だけで使用してください。

## 文字コード

SerialとTelnetは、`utf-8`、`shift_jis`、`euc-jp`、`iso-2022-jp`に対応しています。SSHの文字コードは、接続先ごとのsshc設定に保存できます。

## 自動処理から使う

```sh
sshc serial /dev/ttyUSB0 --non-interactive \
  --expect 'login:' --timeout 20s -- 'admin'

sshc telnet 192.0.2.20:23 --non-interactive \
  --script ./steps.json --json -- ''
```

対話操作を必要としない自動処理では、終了条件として`--expect`または`--read-for`のどちらか1つを指定します。`--expect`は正規表現に一致した時点、`--read-for`は指定時間の読み取りを終えた時点で成功します。全体の`--timeout`を超えた場合は終了コード124、引数が不正な場合は2、通信などほかの失敗は1、割り込みは130を返します。

待機時間、最大受信量、改行コード、出力が1バイト以上必要かどうかも指定できます。受信量は設定した上限で打ち切られ、JSONモードでは結果と警告が1つのオブジェクトとして返ります。

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

データビット、パリティ、ストップビット、フロー制御（`none`、`rtscts`、`xonxoff`）を接続時に指定できます。`--dtr on|off`と`--rts on|off`は接続直後に一度だけ適用し、`--break <期間>`は接続直後に指定した長さのブレークを一度送ります。接続中に切り替える操作はありません。これらの指定はフロー制御の設定とは別です。

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
  --script ./steps.json --json
```

`--script`は手順ファイルの各ステップが送信文と終了条件を持つため、`-- <text>`や`--expect`／`--read-for`とは併用できません。

対話操作を必要としない自動処理では、終了条件として`--expect`または`--read-for`のどちらか1つを指定します。`--expect`は正規表現に一致した時点、`--read-for`は指定時間の読み取りを終えた時点で成功します。全体の`--timeout`を超えた場合は終了コード124、引数が不正な場合は2、通信などほかの失敗は1、`Ctrl+C`やSIGTERM・SIGHUPで止めた場合は、接続の途中でも130を返します。

待機時間、最大受信量、改行コード、出力が1バイト以上必要かどうかも指定できます。受信量は設定した上限で打ち切られ、JSONモードでは結果と警告が1つのオブジェクトとして返ります。

## フラグの一覧

Serialの接続:

| フラグ | 値 |
|---|---|
| `--baud N` | 通信速度。1〜4000000（既定は9600） |
| `--data-bits N` | データビット。5、6、7、8（既定は8） |
| `--parity P` | パリティ。`none`、`odd`、`even`、`mark`、`space` |
| `--stop-bits N` | ストップビット。`1`、`1.5`、`2` |
| `--flow F` | フロー制御。`none`、`rtscts`、`xonxoff` |
| `--dtr on\|off`、`--rts on\|off` | 接続直後に一度だけ信号線を設定する |
| `--break D` | 接続直後に、指定した長さ（5秒まで）のブレークを一度送る |

Telnetの接続:

| フラグ | 値 |
|---|---|
| `--connect-timeout D` | 接続を待つ時間（既定は10秒） |
| `--terminal-type TYPE` | サーバーへ伝えるターミナルの種類（1〜64バイト） |

共通:

| フラグ | 値 |
|---|---|
| `--encoding NAME` | 文字コード。`utf-8`、`shift_jis`、`euc-jp`、`iso-2022-jp` |
| `--non-interactive` | 自動処理として動かす |
| `--expect REGEX` | 出力がこの正規表現に一致したら成功する |
| `--read-for D` | 指定した時間だけ読んで成功する |
| `--script FILE\|-` | 手順ファイル（`-`は標準入力）の各ステップを順に送る |
| `--timeout D` | 全体の上限時間。超えると終了コード124 |
| `--settle D` | 出力が途切れてから、続きが無いと見なすまでの時間（0〜5秒、既定は120ミリ秒） |
| `--max-bytes N` | 読む量の上限（既定は1 MiB、最大16 MiB） |
| `--line-ending MODE` | 送る文の後ろに付ける改行。`none`、`cr`、`lf`、`crlf` |
| `--require-output` | 出力が1バイトも無ければ失敗にする |
| `--json` | 結果をJSONで出す。`sshc serial --json`はデバイスの一覧をJSONで出す |

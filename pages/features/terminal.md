---
title: 接続とTerminal
description: sshcのブラウザTerminal、接続状態、検索、文字コード、port forwarding。
---

# 接続とTerminal

![接続中のTerminalと操作メニュー](/images/terminal-desktop.png)

## 接続の状態を見失わない

名前解決、踏み台接続、host key確認、認証、shell開始、再接続、終了を段階として表示します。利用者の確認が必要なhost keyや認証失敗を、無条件には再試行しません。

終了したSSH sessionは同じpaneとscrollbackのまま再接続できます。利用者が閉じたSSH／local shellは即座に強制停止し、一覧から取り除きます。

## Terminal操作

- `Ctrl/Cmd+F`によるscrollback検索
- URLとremote pathの操作、OSC 8 link
- Quick CommandsとSnippet
- OSC 52 clipboard、Kitty keyboard protocol、JIS keyboard
- 接続ごとのUTF-8、Shift_JIS、EUC-JP、ISO-2022-JP
- 16 KiB〜4 MiBのscrollback上限とfont size設定

scrollbackはmemoryだけに保持し、vault、backup、sync snapshotへ保存しません。

## Port forwarding

SSH接続ごとにLocal forwardingとDynamic SOCKSを管理します。Localはlocal bind address／portと接続先host／portを指定し、Dynamicはlocal SOCKS endpointを開きます。Remote forwardingは提供しません。

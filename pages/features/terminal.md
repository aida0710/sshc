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
- 大文字小文字、正規表現、全一致highlightと検索結果移動
- URLとremote pathのcontext action、OSC 8 link
- Quick CommandsとSnippet
- OSC 52 clipboard、Kitty keyboard protocol、JIS keyboard
- 接続ごとのUTF-8、Shift_JIS、EUC-JP、ISO-2022-JP
- 16 KiB〜4 MiBのscrollback上限とfont size設定
- WebGL rendererとcanvas fallback

scrollbackはmemoryだけに保持し、vault、backup、sync snapshotへ保存しません。

## 入力とClipboard

選択時copyと右click pasteはSettingsで切り替えます。OSC 52は全体の既定値に加え、SSH接続ごとに許可／拒否を保存できます。remoteからclipboardへ書き込める機能なので、信頼できる接続だけで有効にしてください。

Kitty keyboard protocolはremote applicationの要求に合わせて切り替わります。JIS keyboardでは円記号keyをbackslashとして送る設定を利用できます。mobileではCtrl、Alt、Esc、Tab、矢印などの特殊key rowを表示します。

## Linkとremote path

OSC 8 linkと画面上のURLはsystem browserで開けます。検出したremote pathは、そのsessionと同じ接続先を選んだSFTP画面で開けます。

## Local shell

Local shellはSSHと同じTerminal subsystemとして扱われます。Settingsでshell profileを選択でき、Workspace、検索、Quick Commands、一括commandの対象にできます。

## Coding Agent連携

別repositoryの[sshc-agent-bridge](https://github.com/aida0710/sshc-agent-bridge)を明示的に導入すると、Claude Code、Codex、OpenCodeの作業中、入力待ち、完了をpane headerへ表示できます。完了／確認待ちはbackground notificationの対象にでき、Agent自身のsession IDがある場合だけ同じpaneまたは新しいpaneで再開します。

連携はopt-inです。導入しない通常のshell出力を解析して、任意commandを自動再実行することはありません。

## Port forwarding

SSH接続ごとにLocal forwardingとDynamic SOCKSを管理します。Localはlocal bind address／portと接続先host／portを指定し、Dynamicはlocal SOCKS endpointを開きます。Remote forwardingは提供しません。

詳しい設定は[Port forwarding](/terminal/port-forwarding)を参照してください。

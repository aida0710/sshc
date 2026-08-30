---
title: Workspace
description: SSHとlocal shellを最大4 paneへ分割し、配置を保存する。
---

# Workspace

![4つのTerminalを開いたWorkspace](/images/workspace-desktop.png)

接続済みTerminalを表示中のpaneへDrag & Dropすると、drop位置に合わせて上下左右へ分割します。SSHとlocal shellを区別せず、一つのWorkspaceとして扱います。

## Layout

- 最大4 pane
- dividerのDragで比率を10〜90%へ変更
- paneの入れ替え
- 単一paneへ集中するFocus Mode
- 名前を付けてlayoutを端末内へ保存

保存するのはpane種別、接続先、分割木、比率、focusだけです。session ID、remote process、scrollbackは保存しません。Homeから開き直すと、新しいSSH sessionまたはlocal shellを開始します。

## Command Center

実行対象と展開後commandをpreviewした後、接続中の全paneへcommandとEnterを送れます。SSHとlocal shellが混在していても、現在のcwd、environment、shell状態を維持したまま各PTYへ入力します。

mobileでは狭い画面へ分割を並べず、Workspace内のpaneを一つずつ切り替えます。

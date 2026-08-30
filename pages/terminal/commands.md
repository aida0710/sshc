---
title: Quick CommandsとSnippet
description: commandを保存、previewし、単一paneまたはWorkspaceへ送る。
---

# Quick CommandsとSnippet

Quick CommandsはTerminalのoverflow menuから開き、保存済みSnippetを現在のpaneへ挿入、実行、copyできます。

![TerminalのQuick Commands menu](/images/terminal-actions.png)

## Snippetを作る

MenuのSnippetsで名前、command、変数を保存します。secret変数は通常の変数と分けて扱い、library自体もVaultのmaster keyで暗号化します。

## 実行前に確認する

実行はpreviewと実行の2段階です。previewには対象alias、展開後command、必要な入力が表示されます。preview後にsessionのprocessが入れ替わった場合は送信を拒否し、別processへの誤送信を防ぎます。

## Workspaceへ一括送信

2 pane以上のWorkspaceではCommand Centerを開き、ad-hoc commandまたはSnippetを選択できます。対象を選び、previewを確認してから各PTYへcommandとEnterを送ります。SSHとlocal shellを区別しません。

::: warning Secret
Terminalへ送信した値はremote shell history、TTY echo、scrollbackへ残る可能性があります。secret変数をlive Terminalへ送る操作は拒否されます。
:::

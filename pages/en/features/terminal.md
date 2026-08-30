---
title: Connections and terminal
description: Browser terminals, connection stages, search, encodings and port forwarding.
---

# Connections and terminal

![A connected terminal and its action menu](/images/terminal-desktop.png)

## See connection progress

sshc displays name resolution, jump hosts, host-key checks, authentication, shell startup, reconnect and exit as distinct stages. It does not blindly retry host-key or authentication failures that need a decision.

Reconnect an exited SSH session in the same pane while retaining its scrollback. Closing an SSH or local shell explicitly stops it immediately and removes it from the list.

## Terminal controls

- Scrollback search with `Ctrl/Cmd+F`
- URL and remote-path actions, including OSC 8 links
- Quick Commands and snippets
- OSC 52 clipboard, Kitty keyboard protocol and JIS keyboards
- Per-connection UTF-8, Shift_JIS, EUC-JP and ISO-2022-JP
- Configurable 16 KiB–4 MiB scrollback limit and font size

Scrollback stays in memory. It is not written to the vault, backups or sync snapshots.

## Port forwarding

Manage Local forwarding and Dynamic SOCKS per SSH connection. Local forwarding has a local bind endpoint and a destination host and port. Dynamic forwarding opens a local SOCKS endpoint. Remote forwarding is intentionally not provided.

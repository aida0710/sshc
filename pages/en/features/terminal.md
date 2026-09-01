---
title: Terminal
description: Open SSH and local shells with search, reconnect, encodings, and port forwarding.
---

# Terminal

Open SSH and local shells in the same interface, with multiple-pane workspaces, search, Quick Commands, and port forwarding.

![A connected terminal and its action menu](/images/terminal-desktop.png)

## See connection progress

sshc displays name resolution, jump hosts, host-key checks, authentication, shell startup, reconnect and exit as distinct stages. It does not blindly retry host-key or authentication failures that need a decision.

Reconnect an exited SSH session in the same pane while retaining its scrollback. Closing an SSH or local shell explicitly stops it immediately and removes it from the list.

## Terminal controls

- Scrollback search with `Ctrl/Cmd+F`
- Case, regex, match highlighting, and result navigation
- URL and remote-path context actions, including OSC 8 links
- Quick Commands and snippets
- OSC 52 clipboard, Kitty keyboard protocol and JIS keyboards
- Per-connection UTF-8, Shift_JIS, EUC-JP and ISO-2022-JP
- A 16 KiB–4 MiB engine replay buffer, 1,000–100,000 lines of browser scrollback, and configurable font size
- WebGL rendering with a canvas fallback

The engine replay buffer and browser scrollback both stay in memory. They are not written to the vault, backups, or sync snapshots.

## Input and clipboard

Normal text-selection copying stays inside the browser and does not require OSC 52. Configure automatic copy-on-select and right-click paste under **Settings → Terminal**. OSC 52 lets remote software write to the device clipboard. It has a global default and a per-SSH-host allow/deny override, so enable it only for hosts you trust.

Kitty keyboard mode follows requests from the remote application. A JIS option sends the yen key as backslash. Mobile adds a special-key row for Ctrl, Alt, Esc, Tab, and arrows.

OSC 8 links and detected URLs open in the system browser. A detected remote path can open SFTP at the same host and directory.

Local shells use the same subsystem as SSH: search, Quick Commands, workspaces, and broadcast commands all apply.

## Coding Agent integration

After explicitly installing [sshc-agent-bridge](https://github.com/aida0710/sshc-agent-bridge), pane headers can show working, attention, and completion states from Claude Code, Codex, and OpenCode. Background attention/completion may trigger notifications. When the agent provides its own session ID, you can explicitly resume it in the same or a new pane.

The integration is opt-in. Without it, sshc does not infer agent state from ordinary shell output or automatically rerun arbitrary commands.

## Port forwarding

Manage Local forwarding and Dynamic SOCKS per SSH connection. Local forwarding has a local bind endpoint and a destination host and port. Dynamic forwarding opens a local SOCKS endpoint. Remote forwarding is intentionally not provided.

See [Port forwarding](/en/terminal/port-forwarding) for setup details.

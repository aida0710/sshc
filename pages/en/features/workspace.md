---
title: Workspaces
description: Group terminal sessions into named workspaces, and keep connections opened by LLMs in a hidden one.
---

# Workspaces

![Sessions grouped by workspace](/images/terminal-desktop.png)

The sidebar shows sessions grouped by workspace. Press a workspace row to show or hide its sessions; this browser remembers which ones are folded.

Each session row has a dot for its state, the session name on its first line and the destination on its second, so the name has the whole width.

- **Dot**: green when connected, an amber ring while connecting or reconnecting, faint when it has exited. A filled amber dot means an unread notification
- **Second line**: the destination while connected. While connecting, reconnecting, after it exits, or when there is a problem, the state takes that place ("reconnecting 2/5", "exited 255", and so on)
- **Unread notifications**: a session whose program (Claude Code, Codex, ...) sent a notification shows its text under the destination until you open the session. The count in the "Workspaces" heading, such as **Unread 2**, opens the unread sessions in sidebar order
- **… and ×**: with a mouse, they appear only on the selected row and the row under the pointer

## Built-in workspaces

Two workspaces always exist and cannot be deleted. Both can be renamed.

| Workspace | What goes in | Shown |
| --- | --- | --- |
| Main | Sessions opened from the app, including Home and SFTP | Yes |
| Agents | Sessions opened with `sshc terminal create` | Hidden |

Connections that LLMs and scripts open with `sshc terminal create` go into Agents and stay out of the sidebar. `--workspace` puts them in another workspace instead (see [CLI](/en/reference/cli)).

## Actions

- **Create**: **+** next to the Workspaces heading (**New workspace**)
- **See hidden workspaces**: the heading's **…** → **Show hidden workspaces**. Hidden ones appear dimmed.
- **Open a session**: **+** on a workspace row. **New session** below the tree opens in the selected session's workspace.
- **Move**: drag a session onto another workspace's row, or onto a session inside it. Dropped on the row, it goes to the end of that workspace; dropped on a session, it goes just before it. Folded and empty workspaces take drops too. A session's **…** → **Move to workspace** does the same
- **A workspace's …**: rename, hide or show, [send a command](/en/terminal/commands), delete

Deleting a workspace closes nothing: its sessions move to Main.

## What is stored

The engine keeps workspace names and whether each is hidden in `~/.ssh/sshc/workspaces.json`. Which session is in which workspace lives in the engine's memory, like the sessions themselves. When the engine stops cleanly, the record kept for reopening includes each session's workspace, so sessions [reopened](./terminal#reopen-sessions-after-a-stop) on the next start return to the same workspace. The file is excluded from encrypted sync.

Use [Split view](./split-view) to arrange sessions on screen. Split arrangements are not saved.

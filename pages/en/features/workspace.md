---
title: Workspaces
description: Group terminal sessions into named workspaces, and keep connections opened by LLMs in a hidden one.
---

# Workspaces

The sidebar shows sessions grouped by workspace. Press a workspace row to show or hide its sessions; this browser remembers which ones are folded.

## Built-in workspaces

Two workspaces always exist and cannot be deleted. Both can be renamed.

| Workspace | What goes in | Shown |
| --- | --- | --- |
| Main | Sessions opened from the app, including Home and SFTP | Yes |
| Agents | Sessions opened with `sshc terminal create` | Hidden |

Connections that LLMs and scripts open with `sshc terminal create` go into Agents and stay out of the sidebar. `--workspace` puts them in another workspace instead (see [CLI](/en/reference/cli)).

## Actions

- **Create**: **…** next to the Workspaces heading → **New workspace**
- **See hidden workspaces**: the heading's **…** → **Show hidden workspaces**. Hidden ones appear dimmed.
- **Open a session**: **+** on a workspace row. **New session** below the tree opens in the selected session's workspace.
- **Move**: drag a session onto another workspace's row, or onto a session inside it. Dropped on the row, it goes to the end of that workspace; dropped on a session, it goes just before it. Folded and empty workspaces take drops too. A session's **…** → **Move to workspace** does the same
- **A workspace's …**: rename, hide or show, [send a command](/en/terminal/commands), delete

Deleting a workspace closes nothing: its sessions move to Main.

## What is stored

The engine keeps workspace names and whether each is hidden in `~/.ssh/sshc/workspaces.json`. Which session is in which workspace lives only in the engine's memory, like the sessions themselves, and ends when the engine stops. The file is excluded from encrypted sync.

The split-pane Workspace of earlier versions has been removed, and its saved layouts are not carried over.

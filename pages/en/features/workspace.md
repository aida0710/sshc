---
title: Workspaces
description: Arrange SSH and local shells in up to four panes and save the layout.
---

# Workspaces

![A workspace with four terminal panes](/images/workspace-desktop.png)

Drag a connected terminal onto the current pane to split above, below, left or right according to the drop position. SSH and local shells belong to the same workspace model.

## Layout

- Up to four panes
- Drag dividers between 10% and 90%
- Swap panes
- Focus Mode for one pane
- Save a named layout on this device

The saved record contains pane kinds, targets, the split tree, ratios and focus—not session IDs, remote processes or scrollback. Reopening it from Home starts fresh SSH sessions or local shells.

## Command Center

Preview the targets and expanded command, then send the command and Enter to every connected pane. SSH and local shells can be mixed; each PTY keeps its current directory, environment and shell state.

On mobile, sshc shows workspace panes one at a time instead of squeezing the split layout into a narrow screen.

## Build and reuse a workspace

1. Open at least two terminals.
2. Drag a session from the session list onto the current pane.
3. Drop on the top, bottom, left, or right zone.
4. Drag the divider to tune the ratio.
5. Optionally save a named layout.

With one connection, sshc hides layout and broadcast controls and keeps the pane title and search. Saved layouts reopen from Home as new sessions.

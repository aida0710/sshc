---
title: Split view
description: Keep multiple splits of up to four panes and switch between them in the sidebar.
---

# Split view

Arrange SSH and local shell sessions in up to four panes. You can choose sessions from any workspace on the selected run target.

## Arrange sessions

1. Open at least two sessions.
2. Drag a session from the sidebar to the top, bottom, left, or right of a terminal.
3. Drop it in the highlighted area to add a pane there.

You can also choose **Show in split view on the right** or **Show in split view below** from the **…** menu in a terminal heading or sidebar session row. The heading menu then asks which session to add. These actions split the selected pane. Dropping a session onto a workspace in the sidebar still moves its workspace membership.

## Switch between splits

Splitting a session displayed on its own creates a group under **Split views**, above **Workspaces** in the sidebar. Groups are named **Split 1**, **Split 2**, and so on, in creation order. Numbers are not reused after a group is dissolved. You can keep up to eight groups, with up to four panes each. Splitting within a displayed group adds a pane to that group.

Select a group row to return to its arrangement. Session names appear beneath the row; use the arrow to collapse or expand them. Each grouped session also carries a small split icon and group number in the workspace tree.

Selecting a grouped session in the sidebar opens its group and focuses that pane. Selecting an ungrouped session shows it alone while keeping every group available under **Split views**.

A session can belong to only one split group. Adding it to another group removes it from the original group. If only one pane remains, the original group dissolves. Workspace membership does not change.

## Operate panes

| Action | How |
| --- | --- |
| Resize | Drag a divider, or focus it and use the arrow keys. Ratios range from 10% to 90% |
| Swap | Drag a pane's swap button onto another pane, or select the swap button on each pane in turn |
| Focus one pane | Select the focus button in its heading. Choose **Return to split view** or press Esc to restore the split |
| Close a pane | Select **Close pane** in its heading. The session keeps running and remains in the sidebar |
| Select another pane | Click the pane or select its session in the sidebar. Within the terminal, Ctrl+Alt+Left/Right also moves between panes |

The selected pane has a colored border. Choose **Dissolve** on a group row to remove that group, or close panes until only one remains. Neither action closes the sessions themselves. When a session exits or is deleted, its pane is removed, even from a group that is not displayed. Groups with fewer than two remaining panes dissolve.

## Mobile and persistence

Mobile widths show one pane at a time. If you narrow the window during split view, switch panes using the session names at the top. The sidebar still lets you switch between groups. Widening the window restores the split.

Groups and arrangements are not saved to the server or browser storage. Reloading the browser tab or changing the run target clears split view.

See [Workspaces](./workspace) for grouping sessions in the sidebar.

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

Splitting a session displayed on its own creates a split under **Split views**, above **Workspaces** in the sidebar. Splits are named **Split 1**, **Split 2**, and so on, in creation order. Numbers are not reused after a split is dissolved. You can keep up to eight splits, with up to four panes each. Splitting within a displayed split adds a pane to it.

Select a split's row to return to its arrangement. The small map at the row's left shows how its panes are arranged, and the number at its right is how many sessions it holds. Beneath the row, each session in the split has its own row: a status dot and its name, with the destination under the name. Selecting one opens the split with that pane focused. In the workspace tree, a session in a split shows a small map with its own pane highlighted.

Selecting a session in a split in the sidebar opens its split and focuses that pane. Selecting a session in no split shows it alone while keeping every split available under **Split views**.

A session can belong to only one split. Adding it to another split removes it from the original one. If only one pane remains, the original split dissolves. Workspace membership does not change.

## Operate panes

| Action | How |
| --- | --- |
| Resize | Drag a divider, or focus it and use the arrow keys. Ratios range from 10% to 90%, and no pane gets smaller than its heading and one row of the terminal, or narrower than its heading's buttons |
| Swap | Drag a pane's swap button onto another pane, or select the swap button on each pane in turn |
| Focus one pane | Select the focus button in its heading. Choose **Return to split view** or press Esc to restore the split |
| Close a pane | Select **Close pane** in its heading. The session keeps running and remains in the sidebar |
| Select another pane | Click the pane or select its session in the sidebar. Within the terminal, Ctrl+Alt+Left/Right also moves between panes |

The selected pane has a colored border. Choose **…** → **Dissolve** on a split's row to remove that split, or close panes until only one remains. Neither action closes the sessions themselves. When a session exits or is deleted, its pane is removed, even from a split that is not displayed. Splits with fewer than two remaining panes dissolve.

## Mobile and persistence

Mobile widths show one pane at a time. If you narrow the window during split view, switch panes using the session names at the top. The sidebar still lets you switch between splits. Widening the window restores the split.

Splits and arrangements are not saved to the server or browser storage. Reloading the browser tab or changing the run target clears split view.

See [Workspaces](./workspace) for grouping sessions in the sidebar.

---
title: Split view
description: Arrange open sessions in up to four panes.
---

# Split view

Arrange SSH and local shell sessions in up to four panes. You can choose sessions from any workspace on the selected run target.

## Arrange sessions

1. Open at least two sessions.
2. Drag a session from the sidebar to the top, bottom, left, or right of a terminal.
3. Drop it in the highlighted area to add a pane there.

You can also choose **Show in split view on the right** or **Show in split view below** from a session's **…** menu. These actions split the selected pane. Dropping a session onto a workspace in the sidebar still moves its workspace membership.

## Operate panes

| Action | How |
| --- | --- |
| Resize | Drag a divider, or focus it and use the arrow keys. Ratios range from 10% to 90% |
| Swap | Drag a pane's swap button onto another pane, or select the swap button on each pane in turn |
| Focus one pane | Select the focus button in its heading. Choose **Return to split view** or press Esc to restore the split |
| Close a pane | Select **Close pane** in its heading. The session keeps running and remains in the sidebar |
| Select another pane | Click the pane or select its session in the sidebar. Within the terminal, Ctrl+Alt+Left/Right also moves between panes |

The selected pane has a colored border. Selecting a session outside the split ends split view and shows that session alone. When a session exits or is deleted, its pane closes and the remaining panes expand.

## Mobile and persistence

Mobile widths show one pane at a time. If you narrow the window during split view, switch panes using the session names at the top. Widening the window restores the split.

The arrangement is not saved. Reloading the browser tab or changing the run target clears split view.

See [Workspaces](./workspace) for grouping sessions in the sidebar.

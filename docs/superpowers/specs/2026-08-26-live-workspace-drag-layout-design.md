# Live Workspace drag layout design

Date: 2026-08-26

## Goal

Make an active terminal workspace a direct manipulation surface. A user drags an
already connected SSH console from the navigation onto the left, right, top, or
bottom edge of the visible terminal. The drop creates a split without opening a
second connection. The participating consoles then appear as one collapsible
Live Workspace in navigation.

## Product model

- A **console** owns an SSH process and its terminal stream.
- A **Live Workspace** owns only a layout tree and references existing console
  session IDs. Moving or removing a pane never disconnects its console.
- A **Saved Workspace** is the durable, named form of a layout. It stores aliases,
  split directions, ratios, and focus, then opens fresh connections when restored.
- The first release keeps one Live Workspace on the terminal surface. Standalone
  consoles remain selectable without destroying that workspace.

This separation keeps connection lifetime independent from presentation. It also
prevents dragging a connected console from silently creating a duplicate session.

## Interaction

1. A console row publishes its session ID through a private drag MIME type.
2. Every visible SSH terminal exposes four edge zones derived from pointer position.
3. Drag-over paints the prospective half of the target pane.
4. Drop removes the source pane from its old location when necessary, inserts it
   beside the target, focuses it, and preserves its session ID.
5. Navigation collapses two or more member sessions into a Live Workspace row.
   Expanding the row reveals its member consoles for rename, close, and dragging.
6. Removing a pane dissolves a two-pane Live Workspace or removes only that pane
   from a larger one. It does not close any SSH connection.

Keyboard users can pick a pane handle and then choose another pane to swap them.
Split separators retain arrow-key resizing.

## Responsive behavior

Desktop renders the split tree and supports edge drag, resize, broadcast input,
and Focus Mode. A compact viewport renders only the focused pane and a horizontally
scrollable terminal switcher. Selecting a grouped console from navigation updates
the focused pane, so mobile never compresses several terminals into unusable tiles.

## State and failure rules

- Only SSH consoles may join a workspace; local shells stay standalone.
- Closing a console removes any pane that references its session.
- A layout with fewer than two available sessions stops being a Live Workspace.
- A failed Saved Workspace restore leaves the failed pane visible with its error,
  while successfully opened panes continue to work.
- Live Workspace grouping is runtime UI state; persistence is explicit through
  Save Workspace, which deliberately stores aliases rather than session IDs.

## Verification

- Reducer tests cover external docking and relocation of an existing pane.
- Component tests cover edge preview/drop, grouping callbacks, keyboard movement,
  compact one-pane switching, resize, Focus Mode, and partial restore failure.
- Browser screenshots are checked at desktop and mobile viewports before release.

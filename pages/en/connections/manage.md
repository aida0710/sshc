---
title: Connections and groups
description: Search, organize, and edit a large SSH inventory.
---

# Connections and groups

![Three-column connection management](/images/connections-management.png)

Connections uses a group tree, a host list, and a detail pane. Drag the column dividers, or focus one and use the arrow keys (Shift for larger steps, Home for the minimum, End for the maximum), to change the widths; they are kept in the preset. Narrow screens show one level at a time and respect browser or Android back navigation.

## Find a connection

The selector above the list orders connections by **Name** (the default) or in **Configured order** (the order the configuration files list them). **All** and **Ungrouped** in the group column filter them too.

The search keeps the connections where any of these contains what you typed, in any case:

- the connection name, group or tags
- HostName, User, or the `user@host:port` shown
- the path of the file that defines it, or its Host patterns

Command search (`Ctrl/Cmd+K`) searches hosts, remote files, Snippets, and settings across the app. A host result menu opens its details directly. Type `user@host[:port]` to connect without saving, or to save it as a connection and connect.

## Home

Home Quick Connect prioritizes recently connected hosts, then sorts unused entries by name. Panel/list preference is saved in the browser.

- Connect: double-click a card with a mouse, or tap it once on a touch screen. **Connect** in the card's **…** connects too.
- Open its settings: **Open connection settings** in the card's **…** opens that connection in Connections.
- Narrow the list: the group buttons and the search narrow the connections shown.

Below them, Home shows **Needs attention** only when something needs fixing. It counts errors and warnings only.

| Notice | Opens |
|---|---|
| The SSH configuration files have 〈count〉 problem(s). | **Open SSH Config** |
| 〈count〉 notice(s) about connections or group folders need fixing. | **Open Connections** |
| 〈count〉 change(s) stopped partway. | **Recover changes** |

The **Sync** line under it sums up synchronization in one line ("Remote sync is not configured.", "Last synced 〈time〉 · 〈count〉 files", and so on). **Open sync** opens [Sync](/en/features/sync).

## Groups

Groups are folders: a group's connections live in `~/.ssh/connections/<group>/` and its keys in `~/.ssh/keys/<group>/`. Create one with New group at the top of the group column, and use the … next to a group for New group inside, Rename, Edit, Move up, Move down, Hide in Connections, and Delete…. On a computer, right-clicking a group opens the same menu. Deleting a group asks where its connections go. Drag groups to nest them. Changes update the managed `Include` structure.

Move up and Move down change the order of groups at the same level. Hide in Connections works on a group with no connections directly in it. **Show hidden groups (〈count〉)** below the list brings hidden groups into view, and **Close hidden groups** hides them again. While shown they carry a **Hidden** mark, and **Stop hiding** in their **…** menu restores them.

Selecting a group while no connection is open shows the group's editor on the right. Folders lists the files in the group's folder with their connections, its keys, and the `Include` line in `~/.ssh/config` that reads the folder. User name, Port, Key, and Jump host become defaults for the connections in that group; a value set on a connection itself wins.

When a folder under `~/.ssh/connections/` has no `Include` line, or a group's folder is missing, Folder checks appears above the group column. Make it a group turns a folder without an `Include` line into a group.

In a window narrower than 1280px and on a phone, the group button above the list opens the group list with the same … menus and Folder checks.

In a window narrower than 1280px, the group list appears while you drag a connection or a group's heading in the list. It appears over the navigation, or at the right edge of the window when the navigation is hidden; drop on a group there to move what you are dragging. A group with no connections has no heading in the list, so drag it onto another group inside the list that the group button opens. A phone has no dragging, so use Move to group… to move a connection there.

New group in the form for a new connection creates a top-level group without losing what you have typed.

## Act on several connections

Shift-click chooses a range in the list; Ctrl-click (⌘-click on a Mac) adds or removes one connection. With two or more chosen, a bar above the list moves them to a group or deletes them, and dragging any chosen connection onto a group moves them all. Escape clears the choice.

On narrow screens, press Select above the list or long-press a connection, then use the bar at the bottom.

A move or delete of several connections is saved as one change. If any of the affected files changed elsewhere, nothing is written.

## Create, duplicate, and rename

Leave **Connection name** empty on a new connection to take it from the host name (`10.0.0.5` becomes `10-0-0-5`, `web.example.com` becomes `web`), with a number added if that name is taken. **Connect through** chooses the jump host. A group whose defaults name one passes it on by default; **None (connect directly)** opts out.

Aliases are unique. Create, duplicate, and rename operations are rejected before writing if the target alias already exists.

## Connections used as jump hosts

When other connections go through a connection with `ProxyJump`, renaming and deleting it work like this:

- Rename: the same save rewrites the name in every `ProxyJump` of the configuration files. The preview before saving lists those files too.
- Delete: the confirmation lists the connections that go through it. Once it is deleted, they cannot connect through it.
- A name in **Connect through** under **Advanced** that is not a saved connection gets the note "〈name〉 is not a saved connection. ssh connects to it as a host name." Names with a dot (DNS names and IP addresses) do not.

## Settings without a connection

Deleting the Host block of a connection that had sshc's own settings, such as tags or a colour, leaves those settings behind. When **Settings without a connection** appears on the right of Connections, choose one of these:

- **Re-associate 〈name〉**: attach the settings to another connection, such as the one you renamed it to. If that connection already has settings of its own, remove those first or discard these.
- **Discard 〈name〉 settings**: remove the settings left behind.

Quick Connect is the fast launch surface; Connections is the inventory and editing surface. Its item menu links to the full connection details.

---
title: Connections and groups
description: Search, organize, and edit a large SSH inventory.
---

# Connections and groups

![Three-column connection management](/images/connections-management.png)

Connections uses a group tree, a host list, and a detail pane. Drag the column dividers, or focus one and use the arrow keys (Shift for larger steps, Home for the minimum, End for the maximum), to change the widths; they are remembered per browser. Narrow screens show one level at a time and respect browser or Android back navigation.

## Find a connection

The default order is by name. Search matches alias, address, user, and group. Command search (`Ctrl/Cmd+K`) searches hosts, remote files, Snippets, and settings across the app. A host result menu opens its details directly.

Home Quick Connect prioritizes recently connected hosts, then sorts unused entries by name. Panel/list preference is saved in the browser.

## Groups

Groups are folders: a group's connections live in `~/.ssh/connections/<group>/` and its keys in `~/.ssh/keys/<group>/`. Create one with New group at the top of the group column, and use the … next to a group for New group inside, Rename, Edit, Move up, Move down, Hide in Connections, and Delete…. On a computer, right-clicking a group opens the same menu. Deleting a group asks where its connections go. Drag groups to nest them. Changes update the managed `Include` structure.

Move up and Move down change the order of groups at the same level. Hide in Connections works on a group with no connections directly in it. Show hidden groups below the list brings hidden groups back into view, and Stop hiding in their … menu restores them.

Selecting a group while no connection is open shows the group's editor on the right. Folders lists the files in the group's folder with their connections, its keys, and the `Include` line in `~/.ssh/config` that reads the folder. User name, Port, Key, and Jump host become defaults for the connections in that group; a value set on a connection itself wins.

When a folder under `~/.ssh/connections/` has no `Include` line, or a group's folder is missing, Folder checks appears above the group column. Make it a group turns a folder without an `Include` line into a group.

On narrow screens, the group button above the list opens the group list with the same … menus and Folder checks.

New group in the form for a new connection creates a top-level group without losing what you have typed.

## Act on several connections

Shift-click chooses a range in the list; Ctrl-click (⌘-click on a Mac) adds or removes one connection. With two or more chosen, a bar above the list moves them to a group or deletes them, and dragging any chosen connection onto a group moves them all. Escape clears the choice.

On narrow screens, press Select above the list or long-press a connection, then use the bar at the bottom.

A move or delete of several connections is saved as one change. If any of the affected files changed elsewhere, nothing is written.

## Create, duplicate, and rename

Aliases are unique. Create, duplicate, and rename operations are rejected before writing if the target alias already exists.

Quick Connect is the fast launch surface; Connections is the inventory and editing surface. Its item menu links to the full connection details.

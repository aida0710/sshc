---
title: SFTP
description: Remote file operations and a resumable SFTP Transfer Manager.
---

# SFTP

![The SFTP file browser](/images/sftp-desktop.png)

sshc waits for you to choose a host before connecting. The host picker searches aliases, groups, destination hosts and users, and switches between recently connected and SSH Config group views. It initially opens the login user's home directory reported by the SFTP server, and reuses the saved SSH configuration, host keys and credentials.

Switching hosts immediately clears the previous listing and open file. A delayed response from the previous host is discarded instead of being shown under the new selection.

## File operations

- Navigate, create, rename, chmod and delete remote entries
- Select or drag and drop files and folders for upload
- Download files or folders as ZIP archives
- Edit UTF-8 text files up to 2 MiB with Monaco Editor

Use the leading `..` row to move to the parent directory. The navigation controls move back or forward through visited directories, return to the server home directory, or open the root directory. The current path is a clickable breadcrumb; use its edit control when you need to type a path directly. You can filter the current list by name.

Select one row, or use the checkboxes to select multiple entries, to reveal download, rename, delete, and other relevant actions in the selection toolbar. On desktop, Shift-click selects a range, Ctrl/Cmd-click adds to the selection, and Ctrl/Cmd+A selects all displayed entries. The action menu can invert the displayed selection or copy selected names or full paths. Permission and rename actions remain available for a single selection. Double-click or press Enter to open a folder or edit a text file in a modal without resizing the list. Creation and uploads are grouped in the `+` menu at the upper left.

Sort by name, type, bytes or modified time. Permissions appear below the entry name. The selected host and directory are reflected in navigation state, so a terminal remote-path action can open the same location.

On desktop, switch to `2 panes` to keep two remote hosts or directories open side by side. Each pane remembers its selected host and location on the device. Drag files or folders from one pane to the other, then choose copy or move. Data streams directly between the two SFTP connections without a plaintext local spool file. A move within the same connection uses a server-side rename when possible.

Use **Compare** to recursively inspect the two open directories by metadata such as size, modification time, permissions, and type. The preview distinguishes left-only, right-only, changed, and type-mismatched entries. Select only the entries you want, then copy left to right or right to left. Comparison never deletes entries that exist only on the destination.

![Comparison preview for two remote directories](/images/sftp-compare-en.png)

The pane action menu can open an SSH Terminal at the displayed directory. In the other direction, a remote directory reported by OSC 7 can be opened in SFTP from the Terminal action menu.

Narrow panes emphasize the filename and place permissions, size, and modified time on one metadata line. Phones remain single-pane.

## Transfer Manager

The Transfer Manager is docked below the file list. Files and folders share one queue, with two concurrent transfers by default and a configurable limit from one to eight. Its compact state shows the active count, aggregate progress, and speed; expand it for per-file progress, speed, remaining time, and controls. Its action menu can pause, resume or cancel all transfers, and failed files in a batch can be retried independently.

Regular file uploads and downloads support up to 512 GiB. Initially, files of at least 100 MiB are divided into 32 MiB ranges and transferred over up to four independent SFTP connections. Upload ranges write to a remote temporary file; the engine records completed ranges, verifies the full contents, and atomically renames the file when complete. File downloads use HTTP Range to resume when the retained local prefix still matches the remote revision. Even with an empty queue, expand the Transfer Manager to enter any allowed split threshold, stream count, and chunk size; one stream disables splitting. The engine needs temporary free space roughly equal to the downloaded file so it can preserve a safe resumable snapshot.

The engine Transfer Manager owns the queue and persists it in `~/.ssh/sshc/transfers.json`. Queued, paused, and recoverable jobs are restored after an engine restart. Remote-to-remote transfers run entirely in the engine and continue even when the SFTP view or browser is closed.

The browser or WebView still handles local file I/O for uploads and downloads. Those transfers continue when you navigate away from SFTP within the same application, but byte transfer stops when the browser or WebView closes. After a reload, an upload requires you to select the same local file again. A folder ZIP download restarts from byte zero rather than resuming.

See [Transfer Manager](/en/sftp/transfers) for states, recovery, and cancellation.

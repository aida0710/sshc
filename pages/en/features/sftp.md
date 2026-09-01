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

Use the leading `..` row to move to the parent directory. The navigation controls move back or forward through visited directories, return to the server home directory, or open the root directory. You can filter the current list by name. Select one row, or use the checkboxes to select multiple entries, then use the action menu at the upper right to download or delete them. On desktop, Shift-click selects a range, Ctrl/Cmd-click adds to the selection, and Ctrl/Cmd+A selects all displayed entries. The action menu can invert the displayed selection or copy selected names or full paths. Permission and rename actions remain available for a single selection. Double-click or press Enter to open a folder or edit a text file in a modal without resizing the list. Creation and uploads are grouped in the `+` menu at the upper left.

Sort by name, type, bytes or modified time. Permissions appear below the entry name. The selected host and directory are reflected in navigation state, so a terminal remote-path action can open the same location.

## Transfer Manager

The Transfer Manager is docked below the file list. Files and folders share one queue, with two concurrent transfers by default. Collapse the queue to its header, or use its action menu to pause, resume or cancel all transfers. Per-file progress, speed and remaining time remain available together with individual controls and retrying only the failed files in a batch.

Large uploads use a remote temporary file and an atomic rename after completion. After a disconnect, they can resume from the transferred remote size. File downloads use HTTP Range and resume when the retained local prefix still matches the remote revision.

The engine Transfer Manager owns the queue and its state, while the browser or WebView handles local file I/O. Transfers continue when you navigate away from SFTP within the same application, but byte transfer stops when the browser or WebView closes. After a reload, an upload requires you to select the same local file again. A folder ZIP download restarts from byte zero rather than resuming.

See [Transfer Manager](/en/sftp/transfers) for states, recovery, and cancellation.

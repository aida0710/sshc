---
title: SFTP
description: Remote file operations and a resumable SFTP Transfer Manager.
---

# SFTP

![The SFTP file browser](/images/sftp-desktop.png)

Selecting a host does not open an SFTP connection. sshc connects only after you press **Connect**. Restored tabs show their saved host and path without reconnecting in the background. The host picker searches aliases, groups, destination hosts and users, and switches between recently connected and SSH Config group views. A first connection opens the login user's home directory reported by the SFTP server and reuses the saved SSH configuration, host keys and credentials.

Switching hosts immediately clears the previous listing and open file. A delayed response from the previous host is discarded instead of being shown under the new selection.

Operations on the same host (listing, details, preview, editing, transfers and the `sshc sftp` CLI) reuse the SFTP connection the previous operation used; a connection that goes unused for 60 seconds is closed. Moving between folders therefore does not repeat the SSH handshake or ask for a one-time code again. As with an open Terminal, locking the vault leaves that recent connection alive for those seconds.

## File operations

- Navigate, create, rename, chmod and delete remote entries
- Select or drag and drop files and folders for upload
- Download files or folders as ZIP archives
- Edit UTF-8 text files up to 2 MiB with Monaco Editor

Use the leading `..` row to move to the parent directory. The navigation controls move back or forward through visited directories, return to the server home directory, or open the root directory. The current path is a clickable breadcrumb; use its edit control when you need to type a path directly. You can filter the current list by name.

Select one row, or use the checkboxes to select multiple entries, to reveal download, rename, and the three-dot action menu in the selection toolbar. Choose Delete from this menu. A confirmation modal shows the selected paths before deletion; cancelling leaves the files unchanged. The Delete key opens the same confirmation. On desktop, Shift-click selects a range, Ctrl/Cmd-click adds to the selection, and Ctrl/Cmd+A selects all displayed entries. The action menu can invert the displayed selection or copy selected names or full paths. Rename remains available for a single selection. Permissions can also be changed for multiple selected files and folders. Double-click or press Enter to open a folder or edit a text file in a modal without resizing the list. Closing the editor with unsaved changes asks before discarding them. Creation and uploads are grouped in the `+` menu at the upper left.

Local panes support folder creation, rename, and deletion on the machine running the sshc engine. Deletion bypasses the trash and recursively removes selected folders. Selecting a symlink removes only the link. Deletion and rename of the root, home directory, or folders containing them are refused. sshc temporary files and paths used by transfers are also protected. Deletion is refused if the selection changed after confirmation. Local rename is unavailable on Android.

The editor supports these keys, among others:

- Find: `Ctrl/Cmd+F`
- Replace: `Ctrl+H` (`Cmd+Option+F` on macOS)
- Go to line: `Ctrl+G`
- Toggle line comment: `Ctrl/Cmd+/`
- Indent: `Tab`, and `Shift+Tab` to outdent
- Toggle the mode in which `Tab` moves focus: `Ctrl+M` (`Ctrl+Shift+M` on macOS)
- Suggestions: `Ctrl+Space`. They do not appear on their own while you type.
- The editor's command palette: `F1`, also available from the right-click menu

In the editor, `Tab` indents and focus stays in the editor. To move from the editor to **Save** or **Close** with the keyboard, switch to the mode in which `Tab` moves focus, then press `Tab`. Focus moves only within the editor's dialog. Toggling the mode again makes `Tab` indent again. The mode lasts until the editor is closed, and the next file you open starts with `Tab` indenting. The editor's command palette also toggles the mode with "Toggle Tab Key Moves Focus".

The editor's command palette lists the other commands, such as folding and multiple cursors, with their keys. Pressing `Esc` closes Find or the editor's command palette and leaves the editor open. Find, the editor's command palette and the right-click menu are shown in English.

If the remote file changed after it was opened in the editor, the editor refuses the save and says so. You can then choose either action:

- **Reload from remote** discards your unsaved changes and reads the current contents. It asks before discarding them.
- **Overwrite** asks before discarding the changes made on the remote, then writes the editor's contents over the remote file. If the remote file changes again while you confirm, the editor does not overwrite it and says so.

Sort by name, type, size or modified time. Sizes are shown in KiB, MiB or GiB; the details dialog also gives the exact byte count. Permissions appear below the entry name. The selected host and directory are reflected in navigation state, so a terminal remote-path action can open the same location.

On desktop, drag a tab onto the file list to split the view into two panes. With one pane, dropping on the left or right half moves that tab into a new pane on that side and leaves a blank tab behind. With two panes, dropping on the other pane moves the tab there, and a pane whose last tab leaves closes. There are at most two panes, and the handle between them resizes the split. With the keyboard, select a tab and press `Shift`+`←` or `Shift`+`→` for the same moves. Both panes have their own tab strip and `+` action, with up to eight independent tabs per side. Tabs share one width whatever the host name; when they exceed the pane width they shrink and then scroll, and `+` stays at the right edge. Each tab remembers its host and location on the device, as does the split.

Drag files or folders from the currently visible tab on one side to the visible tab on the other. Between two hosts you choose copy or move; a drop into another directory of the same host moves, as it does in a desktop file manager, and a drop back into the directory the rows came from does nothing. Data streams directly between the two SFTP connections without a plaintext local spool file. A move within the same connection uses a server-side rename when possible.

An uploaded file is given the modification time of its source (browser uploads, the CLI's `put`, host-to-host copies and transfers with the engine's disk). A newly created file gets the server's default permissions (its umask); a replaced file keeps its own.

Symlinks are listed as `name → target`. A link to a directory opens as that directory; a link to a file is what details, preview, edit and download act on. Saving an edited file rewrites the file the link points to and leaves the link in place. A link whose target cannot be found is marked `(not found)` and cannot be opened. Copy, move, delete, compare and transfers with the engine's disk take the link itself and never follow it. A pane showing the engine's disk lists links the same way: a link opens even when its target is an absolute path, but the link itself cannot be transferred with the engine.

![The two-pane SFTP view with independent tabs on each side](/images/sftp-two-pane-en.png)

Use **Compare** to recursively inspect the directories in the currently selected left and right tabs by metadata such as size, modification time, permissions, and type. The preview distinguishes left-only, right-only, changed, and type-mismatched entries. For comparisons between remote tabs, select only the entries you want, then copy left to right or right to left. Comparison never deletes entries that exist only on the destination.

Local tabs can also be compared. Comparisons involving a local tab display differences without modifying files.

![Comparison preview for two remote directories](/images/sftp-compare-en.png)

The pane action menu can open an SSH Terminal at the displayed directory. In the other direction, a remote directory reported by OSC 7 can be opened in SFTP from the Terminal action menu.

On desktop a narrow pane keeps every column and scrolls the table sideways rather than dropping columns. Phones emphasize the filename, place permissions, size and modified time on one metadata line, and show one horizontally scrollable tab strip and the left pane only; tab dragging, the right pane's connection and comparison stay inactive. Returning to desktop brings the right pane and its tabs back.

The remote creation menu offers **Create symbolic link**. Enter a name and a relative or absolute target, including a target that does not yet exist. Existing entries are never overwritten. **Change link target** in the details dialog replaces only the link itself and leaves the target contents unchanged. Servers without safe link replacement support refuse this change.

The listing and details show owner UID and group GID when supplied by the server. **Change owner and group** in the details dialog accepts numeric UID/GID values from 0 to 4294967295 for regular files and directories. Account names and recursive ownership changes are not supported. The server must support the no-follow `lsetstat@openssh.com` extension. Missing attributes, unsupported operations, and insufficient permissions are reported. Ownership changes through symbolic links are refused. If the entry changes during confirmation, refresh the listing and reopen the dialog.

Below the remote listing, the available disk space for the authenticated user and total capacity are shown as exact byte counts. A server without capacity reporting support still provides a usable directory listing.

## Change permissions for selected remote entries

Select remote files and folders with their checkboxes, then choose **Change selected permissions** from the selection toolbar's three-dot menu. Enter separate octal modes for files and folders; the defaults are `644` and `755`. **Apply to folder contents recursively** also applies those modes to each matching type below selected folders.

Choose **Review changes** to validate the targets and open a confirmation modal. It shows the selection count, the file and folder target counts including recursive contents, both modes, and the recursive setting. Choose **Apply** to proceed. Cancelling either modal changes no permissions. Single-entry changes and applying one mode recursively to a single folder remain available.

Symbolic links are never followed or changed. A selection containing a link has no permissions action. Links found inside recursive folders are skipped and counted in the confirmation. Local panes do not offer chmod.

The selection is limited to 200 entries. All selected trees share a 20,000-entry budget, with a depth limit of 32. Changes detected before execution refuse the entire selection. Connection or permission failures after execution starts can leave partial changes. The dialog reports the count of confirmed changes; partial changes are not rolled back automatically. Refresh the listing, inspect permissions, and select only the entries that still need changes.

The server must support `lsetstat@openssh.com` version 1 for no-follow attribute changes. Unsupported servers refuse permission changes, including single-entry changes. SFTP cannot atomically compare an entry type and change its permissions, so concurrent changes made by other clients remain a best-effort boundary.

## Search file names or contents

In a remote pane, choose **Name** or **Content**, enter a query and press Enter to search below the current directory. Name search retains its case-insensitive behavior. Content search finds exact, case-sensitive text across files. Each match shows its path, line number and a short snippet; select a match to open that line in the editor. If the file changed after the search, search again.

Content search reads regular UTF-8 files only. Files with missing type, size or modification-time attributes are also skipped. Links, binary files and files over 2 MiB are skipped. A search reads up to 64 MiB of file contents in total and stops at 200 matching lines, 20,000 entries or 32 directory levels. Results explain unreadable entries and other omissions. Use **Stop search** to cancel the request. Content search is not available for local tabs.

## Compare contents even when size and modification time match

**Compare** defaults to **Metadata**. Choose **Content (SHA256)** to stream regular files from both sides and compare their SHA256 hashes. Different bytes are detected even when size and modification time match. File content comparison ignores modification time and permissions. Both local-to-remote and remote-to-remote comparisons are supported.

Reading both sides uses network traffic and takes longer than metadata comparison. File-content reads are limited to 256 MiB in total, with the existing 20,000-entry traversal budget. Links are not followed. Over-budget and unsupported files are shown as **Not compared**; these entries prevent the result from confirming a whole-folder match. Closing the dialog or changing modes cancels the current reads.

Changes detected before or after reading end the comparison as a conflict. SFTP servers do not necessarily expose file identities, so a replacement preserving all observable metadata may be undetectable. The operation does not take a simultaneous snapshot of the whole directory.

## Transfer Manager

The Transfer Manager is docked below the file list. Files and folders share one queue, with two concurrent transfers by default and a configurable limit from one to eight. Its compact state shows the active count, aggregate progress, and speed; expand it for per-file progress, speed, remaining time, and controls. Its action menu can pause, resume or cancel all transfers, and failed files in a batch can be retried independently.

Regular file uploads and downloads support up to 512 GiB. By default a file travels over one SFTP connection. Raising the connection count in the Transfer Manager settings divides files of at least 100 MiB into 32 MiB ranges and transfers them over that many independent connections. A host that authenticates with a one-time code transfers over one connection whatever the setting. Upload ranges write to a remote temporary file; the engine records completed ranges, verifies the full contents, and atomically renames the file when complete. File downloads use HTTP Range to resume when the retained local prefix still matches the remote revision. Even with an empty queue, expand the Transfer Manager to enter any allowed split threshold, stream count from one to 128, and chunk size; one stream disables splitting. The engine needs temporary free space roughly equal to the downloaded file so it can preserve a safe resumable snapshot.

The engine Transfer Manager owns the queue and persists it in `~/.ssh/sshc/transfers.json`. Queued, paused, and recoverable jobs are restored after an engine restart. Remote-to-remote transfers run entirely in the engine and continue even when the SFTP view or browser is closed.

The browser or WebView still handles local file I/O for uploads and downloads. Those transfers continue when you navigate away from SFTP within the same application, but byte transfer stops when the browser or WebView closes. After a reload, an upload requires you to select the same local file again. A folder ZIP download restarts from byte zero rather than resuming.

See [Transfer Manager](/en/sftp/transfers) for states, recovery, and cancellation.

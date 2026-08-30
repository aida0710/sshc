---
title: SFTP
description: Remote file operations and a resumable SFTP Transfer Manager.
---

# SFTP

![The SFTP file browser](/images/sftp-desktop.png)

sshc waits for you to choose a host before connecting and reuses its saved SSH configuration, host keys and credentials.

## File operations

- Navigate, create, rename, chmod and delete remote entries
- Select or drag and drop files and folders for upload
- Download files or folders as ZIP archives
- Edit UTF-8 text files up to 2 MiB with Monaco Editor

Sort by name, type, bytes, modified time, permissions, and other columns. The selected host and directory are reflected in navigation state, so a terminal remote-path action can open the same location.

## Transfer Manager

Files and folders share one queue, with two concurrent transfers by default. See per-file progress, speed and remaining time, then pause, resume, retry or cancel. A failed batch can retry only its failed files.

Large uploads use a remote temporary file and an atomic rename after completion. After a disconnect, they resume from the transferred remote size. Downloads resume through HTTP Range, and the engine keeps the queue running when you leave the SFTP screen.

See [Transfer Manager](/en/sftp/transfers) for states, recovery, and cancellation.

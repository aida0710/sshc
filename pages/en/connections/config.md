---
title: OpenSSH configuration
description: Inspect and edit ~/.ssh/config, Include, and Match without hiding the source files.
---

# OpenSSH configuration

sshc treats `~/.ssh/config` as the source of truth. It follows `Include` files and exposes concrete `Host` aliases as connections.

## Config Editor

The editor shows loaded files and their Include relationships. UTF-8 text is written to a temporary file before replacement. Outside sshc-managed group markers, comments, blank lines, directive order, and unrelated blocks are preserved whenever possible. Do not edit the region between the line starting with `# >>> sshc groups` and `# <<< sshc groups` directly: its contents are replaced on the next save.

### Files and directories

**Create file** in the heading creates a new configuration file inside `~/.ssh`. To have OpenSSH read it, reference it from an `Include` in `~/.ssh/config`. The "…" menu in the heading of an open file offers:

- **Rename file**: also rewrites the `Include` lines that name this file directly. Wildcard patterns are left alone, with a warning when the new path no longer matches them.
- **Create directory**: creates a directory inside `~/.ssh`. Manage group folders from the group list in Connections.
- **Delete file**: also removes the `Include` lines that name this file directly. A backup is kept, so the file can be restored from History.
- **Delete directory**: only an empty directory can be deleted.

Renaming and deleting wait until unsaved edits are saved. Paths outside `~/.ssh` are refused.

## Four connection tabs

- **Basic**: host, user, port, authentication
- **Advanced**: the jump host (ProxyJump), port forwarding and the other directives
- **Display & terminal**: colour, OS icon, tags, display order, the terminal's palette and encoding, the VPN, and the other sshc-only behavior
- **Resolved values**: effective values, sources, warnings

Clearing a value removes its connection-specific directive and restores OpenSSH inheritance. Renaming, moving to a group, duplicating, the comment, the storage file, deleting, and **Edit as text** for the raw Host block are in the **…** at the top of the connection.

```sh
sshc info <alias>
sshc info <alias> --json
```

This uses the connection resolver without connecting. It never prints passwords, passphrases, `SetEnv` values, or the body of `ProxyCommand`.

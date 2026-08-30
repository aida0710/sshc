---
title: OpenSSH configuration
description: Inspect and edit ~/.ssh/config, Include, and Match without hiding the source files.
---

# OpenSSH configuration

sshc treats `~/.ssh/config` as the source of truth. It follows `Include` files and exposes concrete `Host` aliases as connections.

## Config Editor

The editor shows loaded files and their Include relationships. UTF-8 text is written to a temporary file before replacement. Outside sshc-managed group markers, comments, blank lines, directive order, and unrelated blocks are preserved whenever possible.

## Four connection tabs

- **Basic**: host, user, port, authentication
- **Analysis**: effective values, sources, warnings
- **Advanced**: ProxyJump, directives, raw block
- **sshc**: encoding, OSC 52, and other sshc-only behavior

Clearing a value removes its connection-specific directive and restores OpenSSH inheritance.

```sh
sshc info <alias>
sshc info <alias> --json
```

This uses the connection resolver without connecting. It never prints passwords, passphrases, `SetEnv` values, or the body of `ProxyCommand`.

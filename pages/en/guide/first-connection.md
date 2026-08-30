---
title: First connection
description: Create the vault and open an existing or new SSH connection.
---

# First connection

## 1. Create the vault

Enter a master password twice. The vault encrypts saved account passwords, key passphrases, secret Snippet values, and sync credentials.

::: warning
The master password cannot be recovered. It is not the sync encryption key, and may differ on each device.
:::

## 2. Find or create a host

Concrete `Host` aliases from an existing `~/.ssh/config` appear immediately. Quick access searches names, groups, tags, and `user@host:port`.

To add one, open **Connections → New connection**, then enter an alias, host, user, and port. Add a key, password, or ProxyJump only when needed.

## 3. Check before connecting

- **Check reachability** verifies name resolution, TCP, and host key.
- **Check authentication with saved settings** continues through authentication.
- **Analysis** shows effective values and their source, including `Include` and `Match`.

Confirm unknown host key fingerprints before saving them. sshc rejects a changed saved key.

## 4. Open it

Use **Connect**, tap a Quick access panel, double-click it on desktop, or run:

```sh
sshc ssh <alias>
```

The progress view distinguishes resolution, jump hosts, host key checks, authentication, and shell startup. On failure, use the displayed error code and cause.

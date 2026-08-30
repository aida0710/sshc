---
title: Keys and known hosts
description: Generate, import, edit, and register SSH keys; inspect server identities.
---

# Keys and known hosts

The Keys screen shows private and public keys, fingerprints, algorithms, and paths. It supports generation, import, rename, passphrase editing, public-key copy, and ssh-agent loading.

Managed keys may live under `~/.ssh/keys/...` following the group structure. Make sure `IdentityFile` points to the resolved existing path. Private keys under `~/.ssh` are included in encrypted sync snapshots.

## Install a public key

Search and select multiple remote keys, then install them on a target server. sshc resolves only that target, so warnings from an unrelated alias-specific `ProxyCommand` are not mixed into the operation.

## Known hosts

Sort by host, algorithm, fingerprint, and other columns. Unknown keys require confirmation; changed keys are never overwritten automatically.

::: warning
Do not delete the previous entry until you have independently confirmed why the server fingerprint changed.
:::

---
title: Keys and known hosts
description: Generate, organise, edit, and register SSH keys; inspect server identities.
---

# Keys and known hosts

The Keys screen shows private and public keys under `~/.ssh`, with fingerprints, algorithms, and paths. It supports generation, rename, moving between groups, passphrase editing, public-key copy, and ssh-agent loading. To bring in an existing key, place the file under `~/.ssh` (or `~/.ssh/keys/<group>/`); the screen lists it on the next scan.

Keys generated with a group selected are saved under `~/.ssh/keys/<group>/`; without a group they go directly under `~/.ssh`. Moving a key into a group relocates it the same way. An `IdentityFile` that points to a missing path fails when the key is loaded before connecting.

::: warning
Encrypted sync does not pick only the keys referenced by `IdentityFile`: every regular file under `~/.ssh` that is not excluded is included. The private keys in scope are encrypted on this device together with the rest of the snapshot before upload. Check the actual scope under **Sync → Synced files** or in `.sshcignore`.
:::

## Install a public key

Under **Remote Keys**, pick one public key from `~/.ssh` and add it to the `authorized_keys` of one or more hosts. Before connecting, sshc shows the public key, the login user, and the destination file for each host, and resolves only those targets, so warnings from an unrelated alias-specific `ProxyCommand` are not mixed into the operation.

## Known hosts

Sort by host, algorithm, fingerprint, and other columns. Unknown keys require confirmation; changed keys are never overwritten automatically.

::: warning
Do not delete the previous entry until you have independently confirmed why the server fingerprint changed.
:::

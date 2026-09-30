---
title: Keys and known hosts
description: Generate, organise, edit, and register SSH keys; inspect server identities.
---

# Keys and known hosts

The Keys screen shows private and public keys under `~/.ssh`, with fingerprints, algorithms, and paths. It supports generation, rename, moving between groups, passphrase editing, public-key copy, and ssh-agent loading. To bring in an existing key, place the file under `~/.ssh` (or `~/.ssh/keys/<group>/`); the screen lists it on the next scan.

Keys generated with a group selected are saved under `~/.ssh/keys/<group>/`; without a group they go directly under `~/.ssh`. Moving a key into a group relocates it the same way. An `IdentityFile` that points to a missing path fails when the key is loaded before connecting.

### How a connection reads IdentityFile

When connecting, sshc expands each `IdentityFile` value by the same rules as OpenSSH before opening the key:

- A leading `~` or `~/` becomes the home directory.
- `${NAME}`, such as `${HOME}`, becomes the value of that environment variable in the sshc engine.
- Tokens such as `%h` (target host name), `%r` (remote user), `%p` (port), `%n` (alias), `%d` (home directory) and `%C` (hash of `%l%h%p%r%j`) are expanded for each target.

sshc does not use a key whose `IdentityFile` has one of these forms. The connection goes on and tries the other keys and methods.

- A relative path such as `id_work`. OpenSSH opens it from the directory `ssh` was started in, and an sshc connection has no such directory. Write it as `~/.ssh/id_work` instead.
- Another user's home directory, such as `~other/.ssh/id_work`.
- A token sshc does not expand, such as `%T`.
- `${NAME}` naming a variable that is not set.

With **Settings → Terminal → Connection log** at **Keys, hops and timings (-vv)** or above, the connection log lists each such key and the reason as a setting that is not applied. `sshc info <alias>` shows a `notice identityfile` row.

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

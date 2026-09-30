---
title: Security
description: Local boundaries, vault encryption, host keys, sync and Telnet.
---

# Security

## Local application

The engine serves its Web UI and API on a loopback address. UI URLs are issued on demand rather than logged at startup for long-term reuse. If the saved port is unavailable and the engine moves to another port, sshc revokes previous browser registrations and requires enrolment against the current engine. On Linux, other OS users on the same machine can read the one-time registration URL that `sshc` passes to the browser, so the engine accepts a registration only from a connection owned by the same OS user or root. A process running as the same OS user is assumed to have access to that user's SSH files already.

## Vault

Password protection encrypts credentials, snippets, sync settings and backups using a key derived from the master password. At least four characters are required, with no character-class restrictions. Protection against copied local files depends on password strength. The sync record (`sshc/sync-state.json`) stores the last synchronized vault contents only as a hash keyed with the vault key, so it cannot be used to check guesses about the vault contents. After an update from an earlier release, sshc rewrites the record into this form the first time the vault is unlocked (at engine start for a vault without a password). Passwords are not accepted through command-line arguments or environment variables. Automatic locking defaults to 12 hours and can be configured in Settings.

Without a password, a random unlock secret is stored on the device and the vault opens automatically at startup. Idle locking does not apply. This does not protect against someone who can read both the device secret and ciphertext. The device secret is excluded from sync; remote data keeps using an independent sync key.

## Change history and backups

When sshc saves a change, it keeps the previous files as backups in `~/.ssh/sshc/backups/` and a record of the change in `~/.ssh/sshc/history/`. **History** can restore the earlier contents. Passwords removed from the vault and private keys from before a change also remain in these backups, encrypted.

sshc keeps the backups and records of changes that are within all of these limits:

- the newest 200 changes
- no older than 90 days
- 512 backup files in total, counted from the newest change. This limit is reached when changes that rewrite many files at once, such as receiving a sync or deleting a folder, follow one another. The newest change is kept even if its backups alone exceed 512 files.

The backups and records of changes outside these limits are deleted automatically the next time a change is saved or the engine starts. The backups and record of an interrupted change are kept until it is completed or rolled back. If a change interrupted more than 90 days ago is completed, its backups and record are deleted the next time a change is saved or the engine starts.

## SSH host keys

In an interactive terminal an unknown host key is saved after you confirm it; hosts configured with `StrictHostKeyChecking no` or `accept-new` save it without confirmation, as OpenSSH does. A changed saved key is rejected regardless of that setting. Non-interactive SSH, SFTP and public-key installation require known keys for the final host and every ProxyJump hop.

## ProxyCommand

When SSH configuration contains `ProxyCommand`, sshc runs that command locally while connecting. Treat the SSH configuration and any included files as executable configuration. Use only files that you have inspected and trust.

## Sync

Connection settings, Vault credentials, Snippets, and the SSH keys in scope are encrypted on this device with a dedicated sync key before upload; the storage provider and anyone holding the S3 credentials cannot read them in plaintext. The bucket name, S3 object names, object sizes, and modification times are not encrypted.

Anyone with the ciphertext can try guessing the sync key offline. Use the random key sshc generates rather than a short typed string.

## Terminal data

Scrollback stays in memory and is not persisted or synced. OSC 52 clipboard access is configurable. A secret sent to a remote shell can still reach remote history, TTY echo or terminal output.

## Telnet

Telnet neither encrypts traffic nor authenticates the server. sshc warns before connecting but cannot secure the protocol. Use it only behind another trusted network boundary when credentials are involved.

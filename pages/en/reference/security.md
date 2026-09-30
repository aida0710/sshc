---
title: Security
description: Local boundaries, vault encryption, VPN, host keys, sync and Telnet.
---

# Security

## Local application

The engine serves its Web UI and API on a loopback address. UI URLs are issued on demand rather than logged at startup for long-term reuse. If the saved port is unavailable and the engine moves to another port, sshc revokes previous browser registrations and requires enrolment against the current engine. On Linux, other OS users on the same machine can read the one-time registration URL that `sshc` passes to the browser, so the engine accepts a registration only from a connection owned by the same OS user or root. A process running as the same OS user is assumed to have access to that user's SSH files already.

## Vault

The vault key encrypts:

- account passwords, key passphrases and TOTP setup keys used for SSH connections
- VPN profile secrets (WireGuard and OpenVPN configuration files, which contain keys, VPN passwords, IPsec pre-shared keys, and the TOTP secret for OpenConnect's second factor)
- snippets, sync settings and backups

With password protection, that key is derived from the master password. At least four characters are required, with no character-class restrictions. Protection against copied local files depends on password strength. The sync record (`sshc/sync-state.json`) stores the last synchronized vault contents only as a hash keyed with the vault key, so it cannot be used to check guesses about the vault contents. After an update from an earlier release, sshc rewrites the record into this form the first time the vault is unlocked (at engine start for a vault without a password). Passwords are not accepted through command-line arguments or environment variables. Automatic locking defaults to 12 hours and can be configured in Settings.

Without a password, a random unlock secret is stored on the device and the vault opens automatically at startup. Idle locking does not apply. This does not protect against someone who can read both the device secret and ciphertext. The device secret is excluded from sync; remote data keeps using an independent encryption key.

## VPN

[Per-connection VPN](/en/features/vpn) starts one Docker container per VPN profile and connects to the VPN inside it. Check the following before using it.

- Anyone who can drive Docker generally holds strong privileges on this machine. Using VPN requires the OS user that runs sshc to be able to drive Docker.
- The container receives only the VPN settings and secrets and the target's address and port. Secrets never appear in command arguments or environment variables; they are written through standard input to an in-memory file (tmpfs) inside the container and disappear when the container stops. For OpenConnect's second factor, only the generated code is passed, never the TOTP secret.
- Settings that are not secrets, such as the type, VPN server and DNS inside the VPN, are stored outside the vault without encryption so that the list can show them while the vault is locked.
- No SSH private key, account password or vault key reaches the container. The SSH handshake, authentication and host-key checks run in the engine on this machine.
- Neither `--privileged` nor `--network host` is used. The devices and capabilities given to the container depend on the type; see [Per-connection VPN](/en/features/vpn).
- Traffic to a target is fail closed. The container rejects packets to the target that would leave through anything but the tunnel, so while the VPN is disconnected nothing reaches the target over Docker's ordinary network.
- The VPN screen's logs and `sshc vpn logs` replace stored secrets with `[REDACTED]`.

## Change history and backups

When sshc saves a change, it keeps the previous files as backups in `~/.ssh/sshc/backups/` and a record of the change in `~/.ssh/sshc/history/`. **History** can restore the earlier contents. Passwords removed from the vault and private keys from before a change also remain in these backups, encrypted.

sshc keeps the backups and records of changes that are within all of these limits:

- the newest 200 changes
- no older than 90 days
- 512 backup files in total, counted from the newest change. This limit is reached when changes that rewrite many files at once, such as receiving a sync or deleting a folder, follow one another. The newest change is kept even if its backups alone exceed 512 files.

The backups and records of changes outside these limits are deleted automatically the next time a change is saved or the engine starts. The backups and record of an interrupted change are kept until it is completed or rolled back. If a change interrupted more than 90 days ago is completed, its backups and record are deleted the next time a change is saved or the engine starts.

## SSH host keys

Host keys are checked against every file in `UserKnownHostsFile` and `GlobalKnownHostsFile`, as OpenSSH does. A host with `HostKeyAlias` is looked up under that name. When the configuration names no file, these are used:

- `UserKnownHostsFile`: `~/.ssh/known_hosts` and `~/.ssh/known_hosts2`
- `GlobalKnownHostsFile`: `/etc/ssh/ssh_known_hosts` and `/etc/ssh/ssh_known_hosts2` on macOS and Linux, `%ProgramData%\ssh\ssh_known_hosts` and `%ProgramData%\ssh\ssh_known_hosts2` on Windows

In an interactive terminal an unknown host key is saved after you confirm it; hosts configured with `StrictHostKeyChecking no` or `accept-new` save it without confirmation, as OpenSSH does, and hosts configured with `yes` (or `true`) do not save it and do not connect. The key is saved to the first `UserKnownHostsFile`. sshc saves only to files inside `~/.ssh`; when that file is elsewhere, such as `/dev/null`, the key is not saved. sshc does not run `KnownHostsCommand`, so hosts that set it are asked about even with `StrictHostKeyChecking no` or `accept-new`.

When the file to save to, such as `~/.ssh/known_hosts`, is a symbolic link, sshc does not write through the link, so a new host does not connect. Hosts already saved still connect. To add a new host's key, connect to the host once with `ssh`. When `~/.ssh` itself is a symbolic link, keys are saved in the folder it points to.

A changed saved key is rejected regardless of that setting. Non-interactive SSH, SFTP and public-key installation require known keys for the final host and every ProxyJump hop.

The Known Hosts screen lists and deletes only the entries in `~/.ssh/known_hosts`. Keys in `~/.ssh/known_hosts2`, in other files named by `UserKnownHostsFile` and in `GlobalKnownHostsFile` are not shown there. If a changed key in one of those files blocks a connection, edit that file directly. When `~/.ssh/known_hosts` is a symbolic link, the Known Hosts screen does not list it.

## ProxyCommand

When SSH configuration contains `ProxyCommand`, sshc runs that command locally while connecting. Treat the SSH configuration and any included files as executable configuration. Use only files that you have inspected and trust.

## Sync

Connection settings, Vault credentials, Snippets, and the SSH keys in scope are encrypted on this device with a dedicated sync encryption key before upload; the storage provider and anyone holding the S3 credentials cannot read them in plaintext. The bucket name, S3 object names, object sizes, and modification times are not encrypted.

Anyone with the ciphertext can try guessing the encryption key offline. Use the random key sshc generates rather than a short typed string.

## Terminal data

Scrollback stays in memory and is not persisted or synced. OSC 52 clipboard access is configurable. A secret sent to a remote shell can still reach remote history, TTY echo or terminal output.

## Telnet

Telnet neither encrypts traffic nor authenticates the server. sshc warns before connecting but cannot secure the protocol. Use it only behind another trusted network boundary when credentials are involved.

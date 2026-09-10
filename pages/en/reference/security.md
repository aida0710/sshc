---
title: Security
description: Local boundaries, vault encryption, host keys, sync and Telnet.
---

# Security

## Local application

The engine serves its Web UI and API on a loopback address. UI URLs are issued on demand rather than logged at startup for long-term reuse. If the saved port is unavailable and the engine moves to another port, sshc revokes previous browser registrations and requires enrolment against the current engine. A process running as the same OS user is assumed to have access to that user's SSH files already.

## Vault

Password protection encrypts credentials, snippets, sync settings and backups using a key derived from the master password. At least four characters are required, with no character-class restrictions. Protection against copied local files depends on password strength. Passwords are not accepted through command-line arguments or environment variables. Automatic locking defaults to 12 hours and can be configured in Settings.

Without a password, a random unlock secret is stored on the device and the vault opens automatically at startup. Idle locking does not apply. This does not protect against someone who can read both the device secret and ciphertext. The device secret is excluded from sync; remote data keeps using an independent sync key.

## SSH host keys

Unknown host keys require confirmation, and changed saved keys are rejected. Non-interactive SSH, SFTP and public-key installation require known keys for the final host and every ProxyJump hop.

## ProxyCommand

When SSH configuration contains `ProxyCommand`, sshc runs that command locally while connecting. Treat the SSH configuration and any included files as executable configuration. Use only files that you have inspected and trust.

## Sync

Snapshots are encrypted on-device with a dedicated sync key before upload. Anyone with bucket credentials can obtain the ciphertext and continue offline guessing, so use a sufficiently long key.

## Terminal data

Scrollback stays in memory and is not persisted or synced. OSC 52 clipboard access is configurable. A secret sent to a remote shell can still reach remote history, TTY echo or terminal output.

## Telnet

Telnet neither encrypts traffic nor authenticates the server. sshc warns before connecting but cannot secure the protocol. Use it only behind another trusted network boundary when credentials are involved.

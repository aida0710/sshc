---
title: Troubleshooting
description: Checks for engine, vault, keys, sync and SSH connection failures.
---

# Troubleshooting

## The engine does not start

```sh
sshc status
sshc engine --replace
```

When the CLI and engine versions differ, stop the old engine and start it from the updated binary. On Android, the failure screen shows the version, error code, detail, Android SDK, device, and ABI, and **Copy diagnostics** copies them together. The report excludes secrets.

If startup stops with `was recorded by an sshc release before v0.24.0`, an interrupted change recorded by sshc before v0.24.0 is still in `~/.ssh/sshc/journal/`. This release can neither complete nor roll back that change. Start the sshc release you used before, choose **Complete** or **Roll back** under **Interrupted transactions** in **History**, and then update again. If you cannot go back to that release, moving the named file out of `~/.ssh/sshc/journal/` lets the engine start, but the change stays half applied, so check the files listed under `path` in that record. If the message says `was recorded by a newer sshc release`, update to the newer release that wrote the record and resolve it there.

## The vault does not unlock

A master password cannot be recovered. If a new vault fails immediately, inspect the error code and detailed cause. `vault_too_new` means the vault format is newer than this binary supports; update sshc and try again.

## A private key is missing after sync

Check whether `IdentityFile` points to the path restored on this device. Keys managed by sshc may be under a group path such as `~/.ssh/keys/...`. Remove stale absolute paths and inspect the resolved path in connection details.

## ProxyJump asks for a password

Every jump host as well as the final host needs the matching saved password or key passphrase. Run the authentication check to identify the failing hop. Prompts that cannot be answered from saved values, such as 2FA, remain visible in the terminal.

## Sync does not advance

```sh
sshc sync
sshc sync now
```

Check that the vault is unlocked, the direction permits the operation, the bucket status is current and a recent check is recorded. `outcome_unknown` means a write may have reached storage but its result was not confirmed. Refresh remote state before deciding whether to retry.

Sync failures show a cause and a stable `Code`. `bucket_authentication_failed` means the access key or secret was not accepted. `bucket_access_denied` means the store returned HTTP 403; because S3-compatible stores may use that status for an invalid signature as well as insufficient permissions, check the credentials, bucket, region and key permissions. `bucket_rate_limited` and `bucket_unavailable` are temporary object-store failures that can be retried. For the fallback `bucket_refused`, check credentials, region and permissions. `bucket_dns_failed` indicates name resolution; `bucket_tls_failed` indicates HTTPS certificate verification or the device clock; `bucket_timeout` and `bucket_unreachable` indicate network reachability. `wrong_passphrase` means the sync key does not match the data at that target, and `passphrase_too_short` that a typed key is shorter than 12 characters. `snapshot_schema_unsupported` means the remote snapshot is in a format this sshc cannot read (older than v6, or newer): if it is newer, update every device that shares the target; if it is older, overwrite it with Force Push from a device allowed to send. `snapshot_rejected` means the snapshot decrypted but failed validation (corrupt or tampered); restore another generation from History or push again from a sending device. It also appears when an earlier sshc release pushed, from Linux or macOS, a file whose name contains a character Windows does not allow in file names, such as `:` or `?`; update sshc on the sending device, push, rename the file that `sync_local_path_unportable` shows, and push again. `snapshot_download_incomplete` and `snapshot_too_large` are a truncated download or the 128 MiB limit. `sync_local_path_unportable` means a file to be sent has a name other machines cannot use as the same name (a Windows reserved name, a name containing a character Windows does not allow in file names such as `:` or `?`, a name ending in a period or space, or a name that differs from another only in letter case); the screen shows its path, so rename it or exclude it in Files to sync. `sync_remote_moved` means another device pushed first, `sync_remote_deleted` that the synced remote is gone, `preview_stale` and `sync_local_changed` that something changed after the preview (review again), and `sync_conflicts` that both sides changed (choose Keep this machine or Take the other machine). Report `sync_internal_failed` with its diagnostic code because it is an unclassified product failure.

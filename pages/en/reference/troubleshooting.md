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

When the CLI and engine versions differ, update the older one and restart the engine: `sshc service restart` restarts an active managed service, and `sshc engine --replace` restarts any other engine. Use `sshc service install` if the service is inactive or its definition is outdated. On Android, the failure screen shows the version, error code, detail, Android SDK, device, and ABI, and **Copy diagnostic details** copies them together. The report excludes secrets.

If saving shows that an interrupted transaction must be completed or rolled back first, or the CLI says `an interrupted change must be completed or rolled back first` (`a pending transaction must be recovered first` in older releases), an earlier change stopped partway and its record is still in `~/.ssh/sshc/journal/`. While the record is there, sshc refuses every write.

1. Under **Interrupted transactions** in **History**, press **View changes**. From the CLI, `sshc open` opens the UI.
2. Compare the before and after of each file. Private keys and Vault files are listed without their contents.
3. Choose **Roll back** to return every file to before, or **Complete** to bring every file to after.

If startup stops with `was recorded by an sshc release before v0.24.0`, an interrupted change recorded by sshc before v0.24.0 is still in `~/.ssh/sshc/journal/`. This release can neither complete nor roll back that change.

1. Start the sshc release you used before.
2. Choose **Complete** or **Roll back** under **Interrupted transactions** in **History**.
3. Update again.

If you cannot go back to that release, moving the named file out of `~/.ssh/sshc/journal/` lets the engine start, but the change stays half applied, so check the files listed under `path` in that record.

If the message says `was recorded by a newer sshc release`, update to the newer release that wrote the record and resolve it there.

## The vault does not unlock

A master password cannot be recovered. If a new vault fails immediately, inspect the error code and detailed cause. What to do for each error code:

- `vault_schema_newer`: a newer sshc saved the vault in a newer format. Update sshc and try again.
- `vault_schema_older`: the vault is in an older format this sshc cannot read. Unlock it once with the sshc release you used before, which moves it to the newer format, and then update to this release.
- `vault_migration_failed`: moving the vault to the newer format failed. The original vault is left as it was before the move. Check the free disk space and the permissions of `~/.ssh/sshc/`, then unlock again.

## A private key is missing after sync

Check whether `IdentityFile` points to the path restored on this device. Keys managed by sshc may be under a group path such as `~/.ssh/keys/...`. Remove stale absolute paths and inspect the resolved path in connection details.

## A key from IdentityFile is not used

sshc does not use a key whose `IdentityFile` is a relative path (`id_work`), another user's home directory (`~other/...`), a token sshc does not expand (`%T`) or `${NAME}` naming an unset variable.

Earlier versions of sshc opened a relative path from the home directory, so a setting such as `IdentityFile id_work` can stop public key authentication after the update. Rewrite it as a path starting with `~` or an absolute path, such as `~/.ssh/id_work`.

Set **Settings → Terminal → Connection log** to **Keys, hops and timings (-vv)** or above to see which keys were not used. [Keys and known hosts](/en/connections/keys) lists the forms that are expanded and the ones that are not used.

## ProxyJump asks for a password

Every jump host as well as the final host needs the matching saved password or key passphrase. Run the authentication check to identify the failing hop. Prompts that cannot be answered from saved values, such as 2FA, remain visible in the terminal.

## Downloads and other folders do not open on macOS

In a pane that shows the engine's files, opening Downloads, Documents, Desktop or a similar folder, or transferring into one, can fail with a message that macOS privacy protection keeps the sshc engine out (`sftp_local_privacy_protection`). macOS does not let a process read or write these folders until you allow it. Allow sshc as follows.

1. Open **System Settings → Privacy & Security → Full Disk Access**.
2. Click **+** below the list, press ⌘⇧G, and enter the path of the sshc executable. For a Homebrew install it is `/opt/homebrew/opt/sshc/bin/sshc` (`/usr/local/opt/sshc/bin/sshc` on an Intel Mac), and for `install.sh` it is `~/.local/bin/sshc`.
3. Restart the engine: use `sshc service restart` for an active service registered by `sshc service install`, or `sshc engine --replace` for any other engine. A vault with a password locks on restart, so unlock it with `sshc vault unlock`.

```sh
sshc service restart
sshc vault unlock
```

Full Disk Access lets sshc read and write the whole disk, not only these folders. If sshc is listed under **Privacy & Security → Files and Folders**, you can allow only the folders you use there instead.

If you run `sshc engine` in a terminal, allow that terminal app (Terminal, iTerm2 and so on) instead of sshc.

The sshc executable is not signed with an Apple Developer ID, so updating sshc can remove the permission. If that happens, remove sshc from the list with **-** and add it again.

## Sync does not advance

```sh
sshc sync
sshc sync now
```

Check that the vault is unlocked, the direction permits the operation, the bucket status is current and a recent check is recorded. `outcome_unknown` means a write may have reached storage but its result was not confirmed. Refresh remote state before deciding whether to retry.

Sync failures show a cause, and **More details** shows a stable code.

| Code | Meaning and what to do |
|---|---|
| `bucket_authentication_failed` | The access key or secret was not accepted |
| `bucket_access_denied` | The store returned HTTP 403. S3-compatible stores may use that status for an invalid signature as well as insufficient permissions, so check the credentials, bucket, region and key permissions |
| `bucket_rate_limited`, `bucket_unavailable` | Temporary object-store failures that can be retried |
| `bucket_refused` | The fallback. Check credentials, region and permissions |
| `bucket_dns_failed` | Name resolution |
| `bucket_tls_failed` | HTTPS certificate verification or the device clock |
| `bucket_timeout`, `bucket_unreachable` | Network reachability |
| `endpoint_must_be_https`, `endpoint_must_have_no_path` | The endpoint is not just an account URL that starts with https://. Put the bucket name in its own field |
| `unsafe_bucket_name`, `unsafe_object_path` | The bucket name or the path in the bucket uses characters other than letters, digits, hyphens, periods and underscores |
| `sync_push_refused`, `sync_apply_refused` | The machine's direction is Receive only or Send only. Change the direction to send or apply |
| `wrong_passphrase` | The encryption key does not match the data at that target |
| `passphrase_too_short` | A typed key is shorter than 12 characters |
| `snapshot_schema_unsupported` | The remote snapshot is in a format this sshc cannot read (older than v6, or newer). If it is newer, update every device that shares the target. If it is older, overwrite it with Force Push from a device allowed to send |
| `snapshot_rejected` | The snapshot decrypted but failed validation (corrupt or tampered). Restore another generation from History or push again from a sending device. It also appears when an earlier sshc release pushed, from Linux or macOS, a file whose name contains a character Windows does not allow in file names, such as `:` or `?`. Update sshc on the sending device, push, rename the file that `sync_local_path_unportable` shows, and push again |
| `snapshot_download_incomplete` | A truncated download |
| `snapshot_too_large` | The 128 MiB limit |
| `sync_local_path_unportable` | A file to be sent has a name other machines cannot use as the same name (a Windows reserved name, a name containing a character Windows does not allow in file names such as `:` or `?`, a name ending in a period or space, or a name that differs from another only in letter case). The screen shows its path, so rename it or exclude it in Files to sync |
| `sync_remote_moved` | Another device pushed first |
| `sync_remote_deleted` | The synced remote is gone |
| `preview_stale`, `sync_local_changed` | Something changed after the preview. Review again |
| `sync_conflicts` | Both sides changed. Choose Keep this machine or Take the other machine |
| `sync_internal_failed` | An unclassified product failure. Report it with its diagnostic code |

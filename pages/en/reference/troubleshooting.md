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

When the CLI and engine versions differ, stop the old engine and start it from the updated binary. On Android, open **Diagnostic details** to see the version, error code, operation and OS information. The report excludes secrets.

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

Sync failures show a cause and a stable `Code`. `bucket_authentication_failed` means the access key or secret was not accepted. `bucket_access_denied` means the store returned HTTP 403; because S3-compatible stores may use that status for an invalid signature as well as insufficient permissions, check the credentials, bucket, region and key permissions. `bucket_rate_limited` and `bucket_unavailable` are temporary object-store failures that can be retried. For the fallback `bucket_refused`, check credentials, region and permissions. `bucket_dns_failed` indicates name resolution; `bucket_tls_failed` indicates HTTPS certificate verification or the device clock; `bucket_timeout` and `bucket_unreachable` indicate network reachability. `wrong_passphrase` means the sync key does not match the data at that target. Report `sync_internal_failed` with its diagnostic code because it is an unclassified product failure.

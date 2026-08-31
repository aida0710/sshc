---
title: Push, pull, and history
description: Understand Git-like sync, automatic checks, conflicts, and force operations.
---

# Push, pull, and history

sshc does not auto-merge snapshots. It compares local and remote revisions and makes the direction explicit.

1. Refresh bucket status to read the live object and history.
2. Review additions, changes, and removals.
3. Push when local is authoritative, or pull when remote is authoritative.
4. Resolve conflicts and removals with an explicit operation.

`sshc sync now` follows the configured direction and performs only decisions that are safe to automate.

Conflict previews show `0600` and `0700` permission differences alongside content changes, separated into the last sync, this machine, and the remote. Files kept locally as `0400` or executable `0500` are checked against their actual permissions before replacement.

When automatic sync is enabled, sshc polls remote state once a minute while the Vault is unlocked, without uploading during the check. Changes made through sshc are pushed once after a five-second quiet period. Changes from external editors are detected by the next remote check and use the same delay. A remote update is handled before any local push.

The configured direction limits automatic work:

| Direction | Remote check | Pull | Push |
|---|---|---|---|
| Bidirectional | Every minute | When safe to apply | Five seconds after the last local change |
| Receive-only | Every minute | When safe to apply | Never |
| Send-only | Every minute; checked again when sending | Never | Five seconds after the last local change |

When receive-only history diverges, automatic receive stops rather than rolling local state back. Review the current remote to see its creation time, source, and changed files, then explicitly accept it as authoritative. Receive-only operations do not write to S3.

Force Push issues a short-lived confirmation token bound to the configured binding generation, target identity, and current live ETag. Inside the operation lock, sshc verifies the same binding and target again before using a conditional PUT against that ETag. A later remote write or a switch to another bucket or path—even one with the same ETag—is therefore rejected without overwriting it.

Force Pull previews conflicts and removals with the remote selected as authoritative. Apply verifies the same ETag and revision again and writes nothing locally if the remote changed.

Bucket history is read directly from S3. The screen initially shows the latest five entries. Expand the list or reveal long S3 object names only when needed. Listing encrypted objects does not require decryption; content diffs and restoration require the sync key.

Existing schema v5 snapshots are automatically read as v6 after decryption and validation. A receive-only device never rewrites S3. The next push from a device allowed to send stores the live snapshot and subsequent history as v6.

```sh
sshc sync
sshc sync push --json
sshc sync push --force --json
sshc sync pull --json
sshc sync pull --force --json
sshc sync now --json
sshc sync auto on
```

`sync pull --force` treats the current remote as authoritative, previews it, and applies only after rechecking the same ETag and revision. `sync push --force` replaces the remote and requires an interactive or equivalent explicit confirmation.

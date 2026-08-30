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

Automatic sync polls remote state once a minute without uploading. Changes made through sshc are pushed once after a five-second quiet period. Changes from external editors are detected by the next remote check and use the same delay. A remote update is handled before any local push.

The configured direction limits automatic work:

| Direction | Remote check | Pull | Push |
|---|---|---|---|
| Bidirectional | Every minute | When safe to apply | Five seconds after the last local change |
| Receive-only | Every minute | When safe to apply | Never |
| Send-only | Before sending | Never | Five seconds after the last local change |

When receive-only history diverges, automatic receive stops rather than rolling local state back. Review the current remote to see its creation time, source, and changed files, then explicitly accept it as authoritative. Receive-only operations do not write to S3.

Force Push issues a short-lived confirmation token bound to the configured target and its current live ETag. The write uses a conditional PUT against that ETag, so a later remote write is rejected without overwriting it. Changing the bucket or path also invalidates the token.

Force Pull previews conflicts and removals with the remote selected as authoritative. Apply verifies the same ETag and revision again and writes nothing locally if the remote changed.

Bucket history is read directly from S3. The screen initially shows the latest five entries. Expand the list or reveal long S3 object names only when needed. Listing encrypted objects does not require decryption; content diffs and restoration require the sync key.

```sh
sshc sync
sshc sync push --json
sshc sync pull --json
sshc sync now --json
sshc sync auto on
```

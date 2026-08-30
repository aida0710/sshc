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

Automatic sync polls remote state but does not upload every minute unconditionally. It compares content digests after changes from sshc or external editors and uploads only when needed.

Force push or pull requires a short-lived token bound to the exact ETag and revision you previewed. A later remote write, or switching buckets, invalidates that confirmation.

Bucket history is read directly from S3. Listing encrypted objects does not require decryption; content diffs and restoration require the sync key.

```sh
sshc sync
sshc sync push --json
sshc sync pull --json
sshc sync now --json
sshc sync auto on
```

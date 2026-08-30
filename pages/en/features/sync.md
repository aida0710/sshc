---
title: Encrypted sync
description: Encrypted snapshots on S3-compatible object storage you provide.
---

# Encrypted sync

![Sync status and change review](/images/sync-desktop.png)

sshc does not provide hosted sync storage or retain your sync data. You choose and configure S3-compatible object storage; sshc encrypts the workspace on your device before storing snapshots there. The storage provider does not receive plaintext.

## First device

1. Enter the bucket, endpoint and access key.
2. Pass the connection check.
3. Generate a sync key when the target is empty.
4. Store the key shown once in a safe place.

## Additional devices

Enter the same bucket path and sync key. When a remote snapshot already exists, sshc saves the configuration only after the key can decrypt it. Use another path if you intend to create a separate dataset.

Choose bidirectional, send-only, or receive-only sync. Receive-only is useful for a secondary read-only device.

## Git-like flow

- Review changes previews local and remote differences.
- Push writes conditionally against the remote ETag last acknowledged by this device.
- Pull applies only when the previewed ETag and revision still match.
- Force Push replaces the remote through a confirmation token bound to the configured target and its current live ETag.
- Force Pull resolves conflicts and removals in favor of the remote, then applies only the previewed ETag and revision.
- History reads earlier snapshots directly from the bucket.

Automatic sync polls the remote once a minute without uploading. Local changes made through sshc trigger one push after a five-second quiet period; unchanged snapshots are not uploaded repeatedly.

::: warning Sync key
Each device may have a different master password. Devices sharing one target must use the same sync key. Losing it makes remote snapshots impossible to decrypt.
:::

See [Push, pull, and history](/en/sync/workflow) for conflicts, force operations, and automatic sync.

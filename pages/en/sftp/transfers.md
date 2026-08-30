---
title: Transfer Manager
description: Queue files and folders, then pause, resume, retry, or cancel them.
---

# Transfer Manager

![The Transfer Manager](/images/transfer-manager.png)

File upload, folder upload, file download, and folder download share one queue. Two transfers run concurrently by default.

Each job shows per-file progress, transferred and total bytes, current speed, remaining time, and a queued/running/paused/completed/failed/canceled state.

- Pause stops new reads and writes while retaining recovery state.
- Resume continues a file transfer when its recovery requirements are met.
- Retry reruns failed files only.
- Cancel ends the job.
- Clear finished removes completed entries from the view.

Uploads use a temporary file in the target directory and atomically rename it on completion. Existing destinations require confirmation. After a reload, select the same local file again. If its name, size, and modified time match and the remote temporary file is still usable, the upload continues from the transferred position.

File downloads resume through HTTP Range when the browser retains the downloaded prefix and its revision still matches the remote file. Folder downloads stream a ZIP and cannot resume from the middle: after a pause, failure, or reload they restart at byte zero. Android hands the completed ZIP to the system file picker.

The engine Transfer Manager owns the queue and its state. Registration, ordering, progress, concurrency, overwrite approval, and recovery checkpoints remain in the engine when the browser or WebView reloads. Multiple open views converge on the same queue.

The browser or WebView still performs local file I/O because only it can access files on the device. Closing it therefore stops byte transfer, but the job is not stranded in browser-only storage. After a reload, uploads require the original local file to be selected again. Stopping the engine also discards its in-memory queue, so transfers must be registered again after an engine restart.

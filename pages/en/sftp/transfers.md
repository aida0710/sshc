---
title: Transfer Manager
description: Queue files and folders, then pause, resume, retry, or cancel them.
---

# Transfer Manager

![The Transfer Manager](/images/transfer-manager.png)

File upload, folder upload, file download, and folder download share one queue. Two transfers run concurrently by default.

Each job shows per-file progress, transferred and total bytes, current speed, remaining time, and a queued/running/paused/completed/failed/canceled state.

- **Pause** stops new reads and writes while retaining resume state.
- **Resume** continues from the remote or local transferred size.
- **Retry** reruns failed files only.
- **Cancel** ends the job.
- **Clear finished** removes completed entries from the view.

Uploads use a temporary file in the target directory and atomically rename it on completion. Existing destinations require confirmation. Folder downloads stream a ZIP; Android hands the result to the system file picker.

The engine owns transfers, so navigating away from SFTP does not stop them. Exiting the engine does; use explicit resume or retry after restart.

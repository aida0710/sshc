---
title: Transfer Manager
description: Queue files and folders, then pause, resume, retry, or cancel them.
---

# Transfer Manager

![The Transfer Manager in English](/images/transfer-manager-en.png)

File upload, folder upload, file download, folder download, and remote-to-remote copy or move share one queue. Two transfers run concurrently by default. The Transfer Manager is docked below the SFTP view and normally shows only the active count, aggregate progress, and speed. Expand it for per-file status and controls.

On desktop, drag the upper grip of the expanded list to resize it. With the grip focused, the arrow, Home, and End keys provide the same control from a keyboard. Height and folded state are saved in the browser. On phones, tap the bottom bar showing counts and progress to open the queue in a separate sheet.

**Transfer settings** is available even when the queue is empty or folded. Its dialog groups speed and recovery, queue and finished transfers, split transfers, and exclusion rules. Number fields apply when focus leaves them; selections and checkboxes apply when changed. Exclusion rules require **Save exclusions**.

Each job shows its name, source → destination, progress, transferred and total bytes, speed, remaining time, status, and available actions. Open the row's details for full paths, the exclusions captured when it started, and failure information.

- Pause stops new reads and writes while retaining recovery state.
- Resume continues a file transfer when its recovery requirements are met.
- Retry reruns failed files only.
- Cancel ends the job.
- Clear finished removes completed and canceled entries from the view.
- Remove from list dismisses one completed, canceled, or failed entry. Clear failed dismisses failed entries without clearing successful history.
- **Auto-clear finished** set to anything but Keep (30 seconds, 5 minutes, 1 hour) removes completed entries that long after they finish. The setting is stored by the engine and shared by every screen.
- **Stop starting new transfers** lets running transfers finish while queued entries show as Held and do not start; **Resume processing the queue** reverts it.
- Queued entries can be reordered with the ↑↓ controls on their rows; running and finished entries stay where they are.

The overflow menu only appears when at least one bulk action is currently available. A cancel request that races with completion is idempotent and keeps the completed result. If sshc cannot remove an upload's remote temporary file, it keeps the failed entry visible with an explanation so that you can retry cancel or removal after restoring the connection.

With automatic recovery disabled, when a remote-to-remote copy or move, or an upload from a pane that shows the engine's files, is paused or canceled, sshc opens a new SFTP connection to remove the temporary file it was writing at the destination. If the connection is lost, or the host authenticates with a one-time code and refuses a second connection, the temporary file can stay behind. It is not shown in the listing, but while it remains, deleting the folder that holds it or any folder above it, or moving such a folder to another host, fails with `sftp_conflict`. The same applies to a temporary file left behind when an editor save is cut off. A temporary file that has not been written for a day is treated as abandoned and removed together with the folder on the next delete or move, so the failures last one day at most. Only sshc's own temporary files are removed this way; nothing else is deleted.

Uploads use a temporary file in the target directory, verify the complete contents, and atomically rename it on completion. Existing destinations require confirmation. After a reload, select the same local file again. If its name, size, and modified time match and the remote temporary file is still usable, the upload skips ranges already recorded as complete and continues with the remainder.

File downloads resume through HTTP Range when the browser retains the downloaded prefix and its revision still matches the remote file. Folder downloads stream a ZIP and cannot resume from the middle: after a pause, failure, or reload they restart at byte zero. Android hands the completed ZIP to the system file picker.

When the sshc engine runs on Windows, a folder cannot be packaged as a ZIP, and its download fails, if the folder itself or anything inside it has a name that contains `:` or is a Windows reserved name such as `CON`, `NUL`, or `COM1`. Windows cannot store a file under such a name as it is: `:` selects a drive or an alternate data stream, and a reserved name refers to a device. Downloads to a folder on the engine's disk and `sshc sftp get` refuse the same names. An engine on Linux or macOS does not refuse them. If the folder itself or anything inside it has a name that contains `\`, the folder cannot be packaged as a ZIP, whatever platform the engine runs on.

Open **Transfer settings** to edit split-transfer defaults even when the queue is empty. The stream count starts at one, so large uploads and downloads initially use a single SFTP connection without splitting. With two or more streams, files at or above the split threshold (initially 100 MiB) are divided into chunks (initially 32 MiB) and processed over up to that many independent SFTP connections. Enter any integer threshold from 16 to 1024 MiB, stream count from one to 128, and chunk size from 8 to 4096 MiB. The actual connection count is capped by the number of unfinished chunks. The engine persists these settings and shares them with the Web UI and CLI. A file already being transferred keeps the settings it started with when the defaults change. `sshc sftp settings` displays or saves the defaults, while the same options on `get` or `put` override one invocation.

Regular files up to 512 GiB can be uploaded or downloaded. A split upload preallocates a remote temporary file and writes non-overlapping ranges through independent connections; the engine persists completed ranges in its transfer queue. To guarantee that download retries use the same remote contents, the engine first prepares the entire file in its private spool, so keep roughly the file size available there. The spool is `$XDG_CACHE_HOME/sshc/sftp-spool` on Linux (`~/.cache/sshc/sftp-spool` when the variable is unset), `~/Library/Caches/sshc/sftp-spool` on macOS, `%LocalAppData%\sshc\sftp-spool` on Windows, and `sftp-spool` in the app's cache directory on Android. If the spool cannot be used, for example because no user cache directory is known, the download fails with `sftp_spool_unavailable` instead of waiting, and the engine logs the reason when it starts. If the spool has less free space than the file size plus 64 MiB, or runs out while the file is written, the download fails with `sftp_spool_full`. Folder downloads packaged as ZIP use a separate limit.

Folder transfers the engine runs itself (copy or move between hosts, and transfers with a pane that shows the engine's files) and folder deletes fail in these cases:

- A source folder holding more than 20,000 entries fails with `sftp_traversal_limit`.
- A folder to delete that holds more than 200,000 entries, or is nested deeper than 128 levels, fails with `sftp_traversal_limit` before anything is removed.
- On a case-insensitive destination (the APFS and NTFS defaults on macOS and Windows), source names that differ only in case or Unicode normalization, such as `README.md` and `readme.md`, become one name. The transfer fails with `sftp_name_collision` instead of overwriting the file it wrote first, even when overwriting was approved.
- An approved overwrite does not replace an entry of a different kind, such as a file where a folder goes; the transfer fails with `sftp_conflict`.
- A remote-to-remote copy or move of a folder whose destination is the source folder itself or inside it fails with `sftp_target_inside_source` without leaving anything at the destination, also when the same server is saved as two hosts. In that case, if an entry with the same name is already at the destination, you are first asked to approve the overwrite, and the transfer fails after you approve it. A destination that reaches into the source through a symbolic link on its path is detected on servers that follow the links on the way when they check a path, such as the OpenSSH SFTP server.

A remote-to-remote copy or move of a file whose destination is the source file itself fails with `sftp_target_is_source` without changing either of them, also when the same server is saved as two hosts or when the destination's path reaches the source file through a symbolic link on the way. Between two hosts saved for the same server, you are first asked to approve the overwrite, and the transfer fails after you approve it. As with folders, a link on the path is detected on servers that follow the links on the way when they check a path. When the destination itself is a symbolic link to the source file, the transfer does not fail: an approved overwrite replaces the link with a file, and the contents of the file it pointed at are not lost.

For folders and files alike, a destination that overlaps the source is not detected when the two hosts see the same place under different paths, for example when a chroot or a bind mount gives each host a different view, or when a path differs only in case on a case-insensitive server (such as the APFS and NTFS defaults on macOS and Windows). Approving the overwrite of such a move may then lose the source.

The engine Transfer Manager owns the queue and atomically persists it in `~/.ssh/sshc/transfers.json`. Registration, ordering, progress, concurrency, overwrite approval, and recovery checkpoints survive browser or WebView reloads and are restored after an engine restart. Views reconcile with the engine every two seconds, so multiple open views converge on the same queue.

The browser or WebView still performs local file I/O because only it can access files on the device. Closing it therefore stops upload or download bytes, but the job is not stranded in browser-only storage. After a reload, an upload appears as waiting to resume and does not send data until the original local file is selected again.

Remote-to-remote transfers are streamed by the engine through two SFTP connections, so they continue after the browser closes. Each file is written to a temporary sibling and atomically published when complete. A remote job interrupted by an engine shutdown returns to the queue and is automatically retried after startup. It is not retried when the copy or move may already have published the target or removed the source but its terminal result could not be recorded in the device-local queue. Such an entry reports **Check the destination** and offers cancel as its only action, so you can verify the result before dismissing it.

A damaged queue file does not stop the engine from starting. The damaged contents are preserved on the device for diagnosis, the queue starts empty, and the preserved copy is never synced.

## Speed limit

The Transfer Manager's **Speed limit** uses KiB/s: 1 KiB/s is 1,024 bytes per second; 0 is unlimited. The setting is persisted by the engine and applies to running transfers. All jobs, range connections, browser windows and CLI transfers share one budget.

The budget counts file payload, including verification reads, rather than SSH headers or metadata requests. Remote copy counts both received and sent payload. Downloads count both the remote-to-spool and spool-to-browser/CLI legs. ZIP preparation counts uncompressed content; delivery counts ZIP bytes. Bounded read-ahead and SSH buffers mean instantaneous network usage and displayed progress speed can differ from this setting.

| Transfer path | Limited payload |
| --- | --- |
| Browser/CLI uploads | Sequential chunks and parallel ranges written to SFTP |
| Browser/CLI file downloads | Sequential/ranged SFTP preparation and HTTP delivery from the cache |
| Engine-local files ↔ remote | SFTP reads/writes and recovery verification |
| Remote copy/move | Both directions when copying content; rename-only moves use metadata |
| ZIP downloads | Uncompressed file content during preparation and completed ZIP bytes during HTTP delivery |

Sequential upload requests receive the existing bounded HTTP chunk before writing it to SFTP. That incoming browser/CLI-to-engine HTTP traffic itself is not limited; SFTP writes are limited. Parallel range uploads stream their input into the limited SFTP writer.

## Automatic recovery

Enable **Recover after connection loss** and set **Maximum reconnect attempts** to 1–10. Recovery is disabled by default. Backoff starts at one second, doubles, and stops growing at 30 seconds. The queue shows **Waiting to reconnect** and the attempt count. Pause and cancel stop the wait and connection attempt. Stopping queue processing, disabling recovery or shutting down the engine also ends automatic recovery.

| Transfer path | Recovery |
| --- | --- |
| Single engine-local files ↔ remote; remote copy | Recreate SSH connections. Check source metadata and content hash, destination revision and the partial file's entire prefix before copying the suffix. These server jobs continue with the browser closed |
| Browser uploads | Recover while the page still holds the original file, using persisted offsets/completed ranges and destination revision. Reloading requires selecting the same source again |
| Browser file downloads | Resume from a checkpoint of the same immutable spool. If SSH failed during spool preparation, prepare it again from the beginning. Changed content stops automatic recovery |
| Browser ZIP downloads | Restart from the beginning |
| CLI `get`/`put` | Share the speed limit. The CLI does not automatically retry its own connection failures |
| Server folder transfers, moves and deletes | No automatic retry; inspect partial results before a manual resume/retry |

Authentication and host-key refusals, permission failures and revision conflicts are not retried. A lost acknowledgement of publication rename also stops recovery: the destination may already be complete. Inspect the destination before cancelling. Paused server file jobs retain their parts; cancellation or removal cleans up only the unpublished part. Failed cleanup keeps its queue record visible.

Files with missing type, size or modification-time attributes refuse recovery with an error. Resumable server jobs require regular files: the source, destination and part cannot themselves be symlinks. Contents are streamed through fixed-size buffers for verification, which can take time and uses the same speed budget. Browser recovery also needs an open page with access to the engine API. If that API is unreachable or its job state cannot be confirmed, automatic recovery stops.

An engine shutdown leaves a server job that was waiting to reconnect paused after the next startup. A manual resume verifies its saved content and continues from that checkpoint.

CLI settings use the same persisted engine defaults. `--speed-limit` is KiB/s; `--reconnect-attempts 0` disables recovery. Changes also affect other transfers.

```sh
sshc sftp settings --speed-limit 2048 --reconnect-attempts 3
sshc sftp settings --speed-limit 0 --reconnect-attempts 0
```

## Excluding entries from folder transfers

Open **Transfer exclusions** in the Transfer Manager settings and save one pattern per line. No entries are excluded by default. Names such as `.git`, `node_modules` and `*.log` match at any depth inside the selected folder. A matching directory and its contents are omitted without entering that directory. Patterns containing `/`, such as `build/cache`, start at the selected folder.

Only `*` and `?` wildcard syntax is supported. `*` matches any sequence within one path segment and `?` matches one character. `**`, negation with `!`, brackets, backslashes, absolute paths and `.` or `..` segments are unsupported. You can save up to 64 patterns, each up to 512 bytes.

Rules apply to browser folder uploads, folder copies between hosts, folder transfers between a host and the engine's local disk, folder ZIP downloads, and CLI `get --recursive` / `put --recursive`. Explicit single-file selections, moves and deletion are unchanged. Omitted entries are not created at the destination and are not part of a successful transfer. Browser uploads report the omitted entry count; engine folder jobs show their captured exclusion rules.

Settings persist in the engine. A queued folder job retains the rules captured when it was registered, including after settings changes or a restart. Prepared ZIP contents stay unchanged on retry. The CLI snapshots the rules at invocation start; `sshc sftp settings` also displays the current rules.

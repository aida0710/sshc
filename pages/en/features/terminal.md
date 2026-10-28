---
title: Terminal
description: Open SSH and local shells with search, reconnect, encodings, and port forwarding.
---

# Terminal

Open SSH and local shells in the same interface, with search, Quick Commands, and port forwarding.

![A local shell open in a named workspace](/images/terminal-desktop.png)

Use [Split view](./split-view) to operate several sessions side by side.

## See connection progress

sshc displays name resolution, jump hosts, host-key checks, authentication, shell startup, reconnect and exit as distinct stages. It does not blindly retry host-key or authentication failures that need a decision.

When SSH needs an unsaved password, key passphrase, or hidden keyboard-interactive answer, each typed character appears as `*` instead of the value. Backspace and `Ctrl+U` update the mask without leaving the plaintext in scrollback.

**Settings → Terminal → Connection log** shows diagnostics for newly opened connections at the depth of OpenSSH's `-v`, `-vv` and `-vvv`. Each line carries the depth that produced it, such as `[sshc][debug1]`. **Basic details (-v)** shows the target, each hop as it connects, the server banner and the session start. **Keys, hops and timings (-vv)** adds the candidate authentication methods and why one is skipped, each key's type, SHA256 fingerprint and how it was unlocked (no passphrase, saved or typed), keys that could not be read, the server host key fingerprint and which known_hosts line matched (or which lines hold a different key), where the password came from, which method authenticated, the terminal request (columns × rows), the names of environment variables sent and refused, the keepalive interval and count, and settings that are read but not applied. **Everything (-vvv)** also lists the host key algorithms, key exchanges, ciphers and MACs that sshc offers to the server, the TCP local and remote addresses, the agent keys by fingerprint and each keyboard-interactive question. Passwords, passphrases, TOTP codes, setup keys and environment variable values are never logged. `sshc ssh <alias> --non-interactive` uses the same setting.

A dropped SSH session retries automatically with a growing delay. While it waits, **Stop reconnecting** at the top of the pane abandons the loop; the session stays listed as exited so you can reconnect by hand or close it. Reconnect an exited SSH session in the same pane while retaining its scrollback. Closing an SSH or local shell explicitly stops it immediately and removes it from the list.

When the sshc engine stops (an update, a restart of the computer, a manual stop), it records the connection, workspace and working folder of each open session on this machine only. After the next start, once the Vault is unlocked, sshc asks whether to reopen them. **Reopen** opens them again in the same order, each in its working folder when that is known. **Don't reopen** deletes the record, and sshc does not ask again. If the engine stops before you answer, it asks again on the next start. A run target (an sshc engine on an SSH host) works the same way: after it restarts, sshc asks the first time you use that run target with its Vault unlocked. The working folder comes from OSC 7, or from a title of the form `user@host: ~/dir`, which bash sets by default on Debian and Ubuntu. An SSH session with neither, or with a folder whose name contains quotes, `\`, `&` or similar symbols, is reopened in the home directory, because some login shells would read those symbols as part of a command. On Linux the engine reads a local shell's folder directly. Sessions opened with `sshc terminal create` are not reopened. The record holds connection names and folder paths only, never passwords or output, and is never synchronized to other machines. A crash or power loss skips the record. **Duplicate this connection** also starts in the working folder of the original session.

## Terminal controls

- Scrollback search with `Ctrl/Cmd+F`
- Case, regex, match highlighting, and result navigation
- URL and path context actions, including previews and downloads of paths, and OSC 8 links
- Quick Commands and snippets
- Pre-send review for pastes containing line breaks or control characters
- OSC 52 clipboard, Kitty keyboard protocol and JIS keyboards
- Per-connection UTF-8, Shift_JIS, EUC-JP and ISO-2022-JP
- A 16 KiB–4 MiB engine replay buffer, 1,000–100,000 lines of browser scrollback, and configurable font size
- WebGL rendering with an automatic DOM-renderer fallback when unavailable or when a background image is active, plus an option to disable WebGL permanently
- An uncompressed background image library with search, selection, rename, delete, a 1–1024 MiB storage limit, and atomic updates to overall and per-connection references
- Static cursors in Terminal and the SFTP text editor when the operating system requests reduced motion, avoiding distracting rapid animation
- **Keep screen on** under **Global settings** in a terminal's "…" menu keeps the screen from turning off while Terminal is shown (Screen Wake Lock API). It applies to every terminal and is saved in this browser only. It appears only when the browser opened sshc over HTTPS or `localhost`

On reconnect, sshc replays only bytes after the browser's last rendered position. Existing output is not duplicated, and older scrollback retained only by the browser is not cleared. If a long disconnect let required output fall out of the engine buffer, the terminal reports the gap. Both buffers stay in memory and are not written to the vault, backups, or sync snapshots.

## Input and clipboard

Normal text-selection copying stays inside the browser and does not require OSC 52. Inside a program that uses the mouse, such as Claude Code, vim or tmux, a drag goes to that program; hold Shift (Option on a Mac) while dragging to select the terminal text instead. Configure automatic copy-on-select and right-click paste under **Settings → Terminal**. A paste containing line breaks, a final Enter, or terminal control characters is not sent immediately. sshc shows the target, logical line count, and a bounded preview with control characters made visible. Edit the text in the review dialog; the line count and warnings update with it. You can cancel, paste the edited text, or remove exactly one final Enter first. Neither the preview nor the original paste is stored. The review stays open while the terminal display changes, and is withdrawn when a reconnect replaces the shell.

OSC 52 lets remote software write to the device clipboard. It is off by default. Turned on, it applies to every terminal, whatever the connection; switch it under **Settings → Terminal** or under **Global settings** in a terminal's "…" menu. Turn it on only while you use connections you trust. Replaying past output does not repeat OSC 52 copies or copy notifications. New copy requests are processed even when they contain the same text.

Kitty keyboard mode follows requests from the remote application. A JIS option sends the yen key as backslash. Mobile adds a special-key row for Ctrl, Alt, Esc, Tab, and arrows. One tap on Ctrl or Alt applies it to the next key only. A quick double tap locks it until you tap it again, and a locked key is shown in the accent colour.

On mobile, a text box below the special keys is always shown. Japanese conversion finishes inside the box, and only the finished text reaches the terminal. Enter starts a new line, and **Send** beside the box sends the text the way a paste does and then presses Enter. To send with Enter instead, turn on “Send with Enter in the phone's text box” in **Settings → Terminal**; Shift+Enter then starts a new line. With it off, Ctrl+Enter (⌘+Enter on a Mac) sends from a hardware keyboard. The Enter that confirms a conversion does not send. Sending an empty box presses Enter alone, for example to confirm a choice in Claude Code. Text with line breaks is sent as one paste when the program in the terminal uses bracketed paste. Otherwise, and whenever the text has control characters, it opens the same review as a paste and is sent only when you accept it. You can still tap the terminal and type into it directly.

OSC 8 links and detected URLs open in the system browser. Ctrl-click (⌘-click on a Mac) opens one at once, except an OSC 8 link whose text is a different address from where it goes: that opens a menu showing the real destination first. A path an SSH session printed (from the preview's “…”) or the current working directory reported through OSC 7 (OSC 9;9 from PowerShell or cmd on Windows) can open SFTP at the same host and directory. SFTP can also open a terminal at its displayed directory: an SSH session for a remote tab, or a local shell on the engine machine for a local tab.

Click a path starting with `/` to show that file over the terminal at once, with no menu to choose from first. PNG, JPEG, GIF, WebP, BMP and SVG images up to 8 MiB are shown. UTF-8 text up to 2 MiB opens in a read-only editor, colored by its file name; Markdown is shown as source. Audio (`.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac` and more) and video (`.mp4`, `.webm`, `.mov` and more) play in the browser’s own player. The engine reads only the parts the player asks for, so seeking in a long video starts at once. Which formats play depends on the browser (HEVC in a `.mov`, for example, may not play in Chrome); when a file cannot play, the reason and **Download** are shown. A file that cannot be previewed, or is too large, still opens with the reason. Any file can be saved to the browser’s machine with **Download**; a file from an SSH session joins the transfer list like any other download. The file is read only when you click its path: from the engine machine for a local shell (from the run target for a shell opened there), and from the host for an SSH session. The image line Claude Code prints when it sends one (`[image] /tmp/….png`) opens the same way. The preview's “…” has **Copy full path**, and for an SSH session also **Browse in SFTP**, **Edit with SFTP** and **Download with SFTP**. Where the file cannot be read, as in an SSH session opened on a run target, clicking the path opens a menu with **Copy link** only.

Long paths that programs such as Claude Code split at the right edge and continue on indented lines are also recognized as a single path.

Names may contain Unicode characters, including Japanese. Enclose paths containing spaces in quotes, such as `"/tmp/画面 1.png"` or `'/tmp/画面 1.png'`, or escape the space with a backslash: `/tmp/画面\ 1.png`. Ambiguous unquoted output is left unlinked to avoid opening only part of a path.

Paths inside backticks and paths immediately after labels or symbols, such as `Saved to:/tmp/x.png`, `保存しました：/tmp/x.png` and `→/tmp/x.png`, are also recognized. So is the path alone in a Claude Code tool line such as `Read(src/app.ts)`, and in Japanese text that runs into it without spaces, such as `画像は~/shots/a.pngに保存しました`. `file:///tmp/画面%201.png` opens the decoded path. File URIs with a host, such as `file://host/…`, are excluded. Trailing `:line:column` and sentence punctuation, including `。`, `、` and `）`, are removed from the path.

A relative path such as `artifacts/shot.png` or `report.md` can be clicked too when its name ends in a common extension: images, text and source code, PDF, audio, video and archives (for example `.png`, `.svg`, `.md`, `.py`, `.json`, `.mp3`, `.mp4` and `.zip`). It is opened from the working folder of the program that printed it. For a local shell on Linux, including one opened on a run target, the engine looks that folder up. For SSH sessions and local shells on macOS and Windows, sshc uses the folder the shell reported through OSC 7, and cannot open the path when the shell reports none.

Paths such as `~/shots/a.png`, `$HOME/shots/a.png` and `${HOME}/shots/a.png` open from the session’s home folder: the engine machine’s home for a local shell, or the home returned by SFTP for a run target or SSH host. If the home folder cannot be read, sshc explains why the path cannot be opened. The `~user/` form is not supported.

A local shell also recognizes `C:\Users\aida\x.png` and `C:/Users/aida/x.png` once the engine is confirmed to run Windows. Windows paths are not linked in SSH sessions or shells on a run target.

While a preview is open, the left and right arrow keys, or **Previous** and **Next**, move to the other files the same terminal printed (paths ending in the extensions above). While text is shown, the arrow keys inside the editor move its cursor. It is not available in an SSH session opened on a run target.

**New session** in the session list opens the same picker SFTP uses for its destination: **Local shell** at the top (one row per detected shell profile), then search, recent connections and groups for SSH hosts. Local shells use the same subsystem as SSH: search and Quick Commands both apply. On macOS and Linux, sshc supplies the terminal information needed by line editors such as zsh and fish even when the engine was started as a background service.

## Images and character widths

SIXEL, iTerm2 inline images (OSC 1337 File=), and the kitty graphics protocol are drawn inside the terminal. Try them with `chafa`, `img2sixel`, `timg`, or `imgcat`. The iTerm2 form accepts PNG, JPEG, and GIF and does not animate. Images are also shown when the file size in bytes (`size=`) is omitted. Kitty graphics support is experimental and shows only images sent as data; sending a file path or shared memory is not supported, and sshc says so when a program asks. Each terminal keeps up to 32 MiB of images and drops the oldest beyond that. A single image may be up to 4096 × 4096 pixels. A kitty image may be up to about 7.5 MB as sent, and a zlib-compressed one (`o=z`) that inflates past its declared size is not shown. When an image sent in chunks is refused partway through, the images already shown in that terminal are cleared as well. Reopening a session redraws images from the retained output; an image the engine could only keep part of is skipped instead of printed as text.

Character widths follow Unicode 11. An emoji takes two cells, so programs that print emoji (Claude Code, for example) stay aligned with the text after them.

## Titles and notifications

Titles set by shells and programs through OSC 0/1/2 become the pane header and the session list name. Notifications sent through OSC 9/99/777 become unread marks, and browser notifications with a sound while the sshc tab is in the background. Progress reported through OSC 9;4 appears as a thin line along the bottom edge of the pane header and its entry in the session list. The list shows progress even while you are viewing another session. See [Titles and notifications](/en/terminal/notifications) for details, including the Claude Code and Codex settings.

sshc never infers program state from ordinary shell output or reruns arbitrary commands on its own.

## Port forwarding

Manage Local forwarding, Remote forwarding and Dynamic SOCKS per SSH connection. Local forwarding opens a port on the engine side and connects from the SSH server. Remote forwarding opens a port on the SSH server and connects from the engine. Dynamic forwarding opens a local SOCKS endpoint.

See [Port forwarding](/en/terminal/port-forwarding) for setup details.

Closing a connection initially focuses **Keep it open**. Press Tab to focus **Close**, then Enter to confirm.

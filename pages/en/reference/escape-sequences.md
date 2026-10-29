---
title: Supported escape sequences
description: The OSC sequences the sshc terminal takes, and the related sequences for images and the keyboard.
---

# Supported escape sequences

These are the OSC (Operating System Command) sequences that sshc acts on when a program in the terminal sends them. The browser's xterm.js draws the screen and handles most sequences; sshc adds titles, notifications, the working directory, the clipboard and more. Those taken by the sshc engine work even for a session no browser has open.

## OSC

| Number | What it does | Taken by | Notes |
| --- | --- | --- | --- |
| 0, 1, 2 | Title | sshc engine | Becomes the pane name, the name in the session list and in command search. Control characters are removed and up to 64 characters are kept. 1 (icon name) is taken as the title too. [Titles and notifications](/en/terminal/notifications) |
| 4, 104 | Change or reset a numbered color | xterm.js | |
| 7 | Working directory (`file://host/path`) | sshc engine, browser | Used where a reopened session starts, for previews of relative paths and to open the folder in SFTP |
| 8 | Hyperlink | browser | When the visible text is an address other than the destination, sshc shows a menu with the destination instead of opening it at once |
| 9 | Notification (iTerm2 form) | sshc engine | Payloads that begin with digits and `;` (ConEmu commands) are not notifications |
| 9;4 | Progress (ConEmu form) | sshc engine, browser | Drawn as a line along the pane heading and its session list entry |
| 9;9 | Working directory (ConEmu form) | browser | For PowerShell and cmd on Windows; used like OSC 7 |
| 10, 11, 12, 110, 111, 112 | Set, query or reset the text, background and cursor colors | xterm.js | |
| 52 | Write to the clipboard | browser | Off by default. A request to read (`?`) gets no answer |
| 99 | Notification (kitty form) | sshc engine | Notifications sent in parts and base64 bodies are supported |
| 133 | Shell integration (A, B, C, D) | sshc engine, browser | Marks where commands start, notifies the end of a command that ran 30 seconds or more, and lets a click at the prompt move the cursor |
| 777 | Notification (rxvt form, `notify;title;body`) | sshc engine | |
| 1337 | iTerm2 inline images (`File=`) | browser | PNG, JPEG and GIF |
| 5522 | Kitty clipboard protocol | browser | Writes are taken only while OSC 52 is allowed, and put text/plain, text/html and image/png on the clipboard. Reads are refused, except a read with the one-time password a paste event (mode 5522) gave, which gets what was pasted |

A notification (9, 99, 777) longer than 4096 bytes keeps its beginning. How unread marks, browser notifications and the sound work is in [Titles and notifications](/en/terminal/notifications).

These OSC sequences are not taken:

- Reading 52 (handing the clipboard's contents to the program)
- 1337 other than `File=` (such as `SetUserVar`)
- 5522 reads other than of a paste, and the primary selection
- 7501 (program status)

## Beyond OSC

| Sequence | What it does | Notes |
| --- | --- | --- |
| DCS (SIXEL) | Images | [Images and character widths](/en/features/terminal#images-and-character-widths) |
| APC `G` (kitty graphics protocol) | Images | Only images sent as data are shown |
| Kitty keyboard protocol | Keyboard | Answers the query and switches as requested |
| Mode 5522 (paste events) | Paste | While a program has it on, a paste is not typed: the program is told which forms were pasted (text/plain, image/png and so on) and given a one-time password to read them |
| Mode 2027 (grapheme clusters) | Character widths | While a program has it on, emoji joined with ZWJ, flags and the like are drawn as one character in two cells. [Images and character widths](/en/features/terminal#images-and-character-widths) |
| Mode 2033, `CSI ? 998 n` (visibility reports) | Whether the terminal is seen | Answered by the sshc engine: visible while any browser shows the terminal, not visible when none does |

Other CSI sequences and modes (mouse, bracketed paste, the alternate screen and so on) follow xterm.js's [feature table](https://xtermjs.org/docs/api/vtfeatures/).

---
title: Settings
description: Global settings, presets, and settings for each machine.
---

# Settings

Settings fall into three groups, and the list on the left of each settings page shows them under these three headings:

- Global: **General**. The same values on every machine, whichever preset it uses.
- Presets: **Appearance**, **Terminal**, **SFTP**, **Keyboard shortcuts**, **Notifications**, and **Vault**. They edit the preset chosen under **Preset to edit** above the list.
- Machines: **Machines**. Each machine that syncs chooses the preset it uses and its port.

Every setting is stored under `~/.ssh/sshc` and synced to your other machines. **Open connections** is not a setting: it closes the connections open on this machine.

Menu lists each page with a line on what it is for, and links directly to the settings pages. Connection-specific behavior lives in the connection's **Display & terminal** tab.

## Presets

To use different values on different machines, such as a larger font on a phone than on a PC, make a preset. A preset holds the appearance, the terminal settings, the SFTP transfer settings, the shortcuts, the notification sound, the Vault auto-lock, keeping the screen on, and touch handling. The navigation and connection-list widths, the Home view and the SFTP split are not part of a preset; each browser remembers its own.

- **Standard** is there from the start. A machine that has not chosen a preset uses Standard.
- The preset this machine uses is marked **In use here**. With another preset open, press **Use on this machine** to have this machine use it. Choose the preset another machine uses under **Machines**.
- **New preset** under "…" copies the values of the preset that is open. **Rename** and **Delete** do not apply to Standard. A machine that used a deleted preset goes back to Standard. Up to 16 presets, Standard included, can be made.
- A change to a preset takes effect on the machines that use it once the change reaches them through Sync.

On a phone, the preset is chosen at the top of each settings page.

## General

Settings that are the same on every machine and every preset. Each change is saved at once.

- **Display language**: **Follow the browser**, English, or 日本語. Preferences in Menu switches it as well
- Terminal connections: **Reconnect after a dropped connection**, **Connection log**, and OSC 52 (allow writing to the clipboard)
- SFTP transfers: how long **Finished transfers** stay in the list, whether to **Resume automatically** **When the connection drops**, and the **Reconnect limit**
- Background images: **Library capacity** (1 to 1024 MiB, 16 MiB by default)

## Machines

Lists the machines that sync, this machine first. A machine joins the list when this version of the sshc engine first starts on it.

- **Preset**: the preset the machine uses. A change takes effect on that machine once it reaches it through Sync.
- **Port**: the port that machine's sshc engine listens on, from its next start.
- **Last start**: when the sshc engine last started on that machine.
- "…": **Rename** and **Remove from the list**. A machine no longer in use can be removed; this machine cannot. A removed machine comes back the next time sshc runs on it.

Desktop uses a device-local stable port so bookmarks and the installed web app keep the same URL. With **Port** left empty, it first tries `127.0.0.1:54447`, then stores an available fallback for that machine if that port is already in use. You may change the port, but must enrol the browser again at the new origin. The Android app's port cannot be changed: the app keeps its saved data per port, so another port would lose the browser's enrolment.

## Keyboard shortcuts

Open **Menu → Keyboard shortcuts** to assign command search, terminal search, copy/paste, previous/next prompt, text size (larger, smaller, reset), quick commands, new session, close session, previous/next connection, previous/next pane of a split, Home and SFTP. Click a binding and press a chord with Ctrl, Alt or Meta (Command), or a function key. Escape cancels; Tab moves to the next control.

Duplicate bindings are rejected. Use **Clear** to disable an action or **Restore defaults** to reset all bindings. Connection switching defaults to Alt+PageDown / Alt+PageUp. Text size defaults to Ctrl+= (also Ctrl+; on a JIS keyboard) for larger, Ctrl+- for smaller and Ctrl+0 to go back to the settings' size (Command on a Mac); like a pinch, it changes only that terminal and is not saved. New session, close session, the panes and quick commands have no default keys: the ones other terminals use (Ctrl+Shift+T, Ctrl+Shift+W, Alt with the arrows) are taken first by the browser or by the program in the terminal. Closing a session asks first, as its × does. Moving to the previous/next prompt defaults to Ctrl+Shift+Up / Down (Command+Up / Down on a Mac): when the shell marks its prompts through OSC 133 (shell integration), it passes over long output to the prompt's line. On mobile, choose it from the terminal's **…** menu. Copy, paste, terminal search, the prompt keys, text size and quick commands apply inside the terminal. Paste retains its normal confirmation and editing flow.

Bindings are kept in shortcut presets: save named ones, duplicate, rename, edit or delete them. Shortcut presets are synced and shared by every machine; each settings preset chooses the one it uses. Editing the default shortcuts creates a personal shortcut preset. Deleting the selected shortcut preset restores the defaults. Reload after a concurrent edit conflict. Browser or OS shortcuts may take priority. Native browser paste remains available.

Paste defaults to Ctrl+V (⌘V on Mac); Ctrl+Shift+V also remains available.

## Appearance

**Settings → Appearance** sets the theme and the accent colour of the whole app in the preset being edited:

- Theme: match the system, light, or dark
- Accent colour: lavender, blue, sky, pink, mono, or custom. It marks the current item, buttons, and the focused terminal pane. Custom takes one colour of your own from the colour picker or as a code (#rrggbb), used in both the light and the dark theme. A colour that is hard to tell from the background of the theme on screen brings up a notice.

Each settings page lists the other settings pages on its left, so you can move between them.

The theme and the accent colour are preset settings and sync to your other machines. The browser also keeps a copy of the theme, so the first screen is drawn in it the next time sshc opens. Amber, green, and red are not among the offered colours because they already mean an unread notification, connected, and an error.

**General**, or Preferences in Menu, switches the display language (Japanese or English).

Configure the terminal color scheme, font, background, and tint under **Settings → Terminal**.

Theme and language controls are also available on vault create and unlock screens.

## Vault

**Settings → Vault** sets the Vault auto-lock and the master password. The auto-lock time is a preset setting. The master password belongs to each machine's Vault and is not synced.

### Auto-lock

By default, the Vault locks after saved passwords and key passphrases have not been used for 12 hours. Select a value from 1 to 999 and either minutes or hours. Status checks, terminal output, and automatic sync do not extend the timer.

With **Do not auto-lock**, the Vault stays unlocked until you lock it manually or restart sshc. Use this option only on a device you control.

A passwordless Vault is neither auto-locked nor lockable by hand: it opens when sshc starts and this setting has no effect. The current-password field is hidden when changing protection; adding a password restores auto-lock, that field, and the lock controls.

## Terminal

**Settings → Terminal** holds the terminal settings of the preset being edited. It is split into the **Display** (color scheme, font, background), **Connection** (limits) and **Behaviour** (starting directory, local shell, clipboard and keyboard, touch input) tabs. On a phone every group is shown as its own section. **Save** stores the values changed on every tab at once; only touch input is saved as soon as it changes. Reconnecting, the connection log and OSC 52 are under **General**.

- Maximum concurrent sessions from 1 to 200, with a default of 50
- Engine replay buffer from 16 KiB to 4 MiB, with a default of 256 KiB
- Browser scrollback from 1,000 to 100,000 lines, with a default of 5,000
- Terminals kept hidden from 1 to 99, with a default of 3. Terminals shown recently stay hidden when they leave the screen, so their whole scrollback is there when you go back. Each keeps its scrollback in the browser's memory (a few MB for 5,000 lines)
- Font size, color scheme, font, background, and tint
- Copy on select and right-click paste
- JIS yen-key to backslash behavior
- Whether Enter sends from the phone's text box (it starts a new line by default)
- Whether to use font ligatures (off by default)
- Whether to show the VT320 status line a program shows (on by default; when off, what is written to it stays off the screen as well)
- With shell integration (OSC 133): command marks, the command kept above its output, and command marks on the scrollbar (all on by default)

The engine replay buffer retains bytes in memory so it can replay output when a browser reconnects. Browser scrollback is the separate number of display lines retained by xterm. Neither setting writes terminal output to disk. When output keeps coming faster than the browser can draw it and the browser falls further behind than the replay buffer holds, the part no longer in the buffer is skipped and the terminal says so.

When a terminal needs output older than the replay buffer holds (one rebuilt after more than the kept hidden terminals were shown, or a new tab), the engine first shows the screen it keeps and then replays what the buffer holds. A program that draws its screen once and then rewrites only what changes, such as Claude Code, is shown with what it drew earlier. Lines from before the replay buffer cannot be scrolled back to.

Encoding may be overridden per connection. OSC 8 and Kitty keyboard behavior are protocol-driven.

Rendering uses WebGL by default and automatically falls back to the DOM renderer when WebGL is unavailable or a background image is active. If a GPU or browser duplicates characters or leaves visual traces, turn off **Use WebGL rendering** to always use DOM rendering.

Turn on **Use font ligatures** to draw `->`, `!=`, `===` and the like as single symbols (off by default). It works with the JetBrains Mono font. A ligature takes as many cells as the characters it replaces, so the cursor and selections stay where they were; where the cursor or a selection edge falls inside one, it is drawn a character at a time.

Select **Change** to open the searchable background image library. The normal settings page shows only the selected image; rename and delete actions live under each image's `…` menu. sshc stores the uploaded bytes unchanged and does not compress or convert the image.

The library capacity is set under **General** (16 MiB by default, 1 to 1024 MiB). Lowering it never removes or recompresses existing images. Large or numerous images increase local storage, encrypted-sync transfer, and memory use.

When an image is renamed, its extension follows the detected format and preset and per-connection references are updated in the same operation. An existing image is never overwritten. Deleting an image clears references to it in the same transaction.

## SFTP

**Settings → SFTP** holds the SFTP transfer settings of the preset being edited: the speed limit, concurrency, split transfers, and transfer exclusions. **Transfer settings** in the SFTP Transfer Manager opens the same page. On the machines that use the preset, browser and CLI transfers share them. Clearing finished transfers and resuming after a lost connection are under **General**. See [Transfer Manager](/en/sftp/transfers) for details.

## Local shell and notifications

Choose a default shell profile and start directory under the **Behaviour** tab of **Settings → Terminal**; both are preset settings. Detected choices include PowerShell variants on Windows and available zsh, fish, bash, and similar shells on Unix systems. **Open Terminal here** from a local SFTP folder uses that default profile with the displayed folder as its start directory. A local shell opened through the usual **New session** action uses the start directory configured here.

Browser notification permission is requested only after an explicit click. Configure, in the preset being edited, the sound and volume used when a terminal program sends a notification through OSC 9/99/777, and test delivery. See [Titles and notifications](/en/terminal/notifications) for details.

## Where settings are stored

Settings are stored under `~/.ssh/sshc` and are included in backups and Sync.

- The global settings and the presets: the sshc metadata (`metadata.json`), together with connection-specific settings
- Each machine's settings (name, preset in use, port): `~/.ssh/sshc/machines/<machine ID>.json`
- Each machine's screen state (the run target of terminals, SFTP tabs and split, the Transfer Manager view, folded workspaces and whether hidden workspaces are shown, and whether transfer processing is stopped): `~/.ssh/sshc/machines/<machine ID>.state.json`

Only `~/.ssh/sshc/machine-id`, which tells the machines apart, is not synced. The browser keeps a copy of the theme and the display language (so the first screen is drawn in them the next time sshc opens), the browser registration, and whether it has shown the result of an update, and nothing else.

The browser registration token is not stored in `~/.ssh` or sync snapshots; the engine keeps only its verification hash, the issue and last-use times, and the engine port. The token is replaced every time the browser re-enters after an engine restart and expires 30 days after its last use. A changed engine port or a presentation of a superseded token revokes the registration, and `sshc` (or `sshc open`) issues a fresh URL to register again. At most 16 browsers are registered; when the limit is reached, the registration unused for the longest is dropped first. The browser session itself ends after 12 hours without use or 7 days after it was issued, but a valid registration re-enters on the next load without asking. "Sign out of this browser" under Menu → Sign out removes both the session and the registration (while the vault is locked, the same button on the lock screen signs out without the master password); opening sshc again then needs the URL from `sshc` (or `sshc open`). Vault secrets and sync credentials are never written to plaintext settings.

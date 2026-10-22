---
title: Titles and notifications
description: Standard escape sequences from shells and coding agents rename panes and deliver attention and completion notifications.
---

# Titles and notifications

The sshc terminal understands the standard escape sequences that programs already send. No dedicated plugin or hook is required. The same mechanism that renames a tab in Terminal.app or iTerm2, and that lets Claude Code and Codex raise notifications, works in sshc's browser terminal.

| Purpose | Sequences | Typical senders |
|---|---|---|
| Rename the pane | OSC 0, OSC 1, OSC 2 | zsh/bash prompt setup, vim, tmux, Claude Code, Codex |
| Raise a notification | OSC 9 (iTerm2 style), OSC 99 (kitty style), OSC 777 (rxvt/tmux style) | Claude Code, Codex, notify commands such as `cmux notify` |
| Show progress | OSC 9;4 (ConEmu style) | `cargo`, `winget`, and other commands that report progress to Windows Terminal or Ghostty |

## Pane titles

When a program sets a title with a sequence such as `ESC ] 0 ; name BEL`, the pane header, the session list, and the command palette all show that name. Titles sent by a remote shell over SSH are recorded by the engine and applied the same way.

- While you have pinned a pane name, program titles never overwrite it. Choose **Use automatic name** to show the program title or the connection alias again.
- When a program clears its title, the pane falls back to the connection alias or shell name.
- A reconnect or restart starts a new shell, so the previous title is dropped until the new shell sends one.
- Control characters are removed and titles are limited to 64 characters.

To try it, run this inside a pane:

```sh
printf '\e]0;hello\a'
```

## Notifications

When a program asks for a notification through OSC 9, OSC 99, or OSC 777, sshc marks that pane as unread.

- A notification that arrives while the sshc tab is in the background, or while you are looking at another session, marks the session in the session list as unread. Showing that session clears the mark.
- While the tab is in the background, sshc also shows a browser notification (if allowed under Settings → Notifications) and plays the selected sound. The browser notification is titled with the pane name; for SSH the connection alias comes first and the pane name follows in parentheses. A program on the server can change the pane name but not the alias, so these stay distinguishable from sshc's own notifications. The body carries the title and text the program sent.
- The Android app shows an Android notification instead of a browser notification. It arrives while sshc is closed (another app or the home screen is showing), and tapping it opens that pane. The Android notification itself is silent; only the selected sound plays. The text is not shown on the lock screen. Next to the app name, the Android notification says it comes from a terminal program.
- A notification for the pane you are currently viewing produces neither an unread mark nor a sound.
- A bare BEL (`\a`) is not treated as a notification because shells ring it for routine events such as failed completion.

To try it, run one of these inside a pane and then switch to another tab:

```sh
printf '\e]9;Task complete\a'
printf '\e]777;notify;Build finished;main.go compiled\a'
```

## Progress

When a program reports progress with `ESC ] 9 ; 4 ; state ; percent BEL`, sshc draws a thin line along the bottom edge of the pane header and its entry in the session list. The list keeps showing progress while you view another session.

- State 1 is normal, 2 is an error (red), 3 is work of unknown length (a line moving back and forth), 4 is paused (yellow), and 0 clears it. Error and paused keep the previous percent when it is omitted.
- The line also clears when the shell reports through OSC 133 that the command finished, and when the session switches to a new shell.

To try it, run this inside a pane:

```sh
printf '\e]9;4;1;40\a'; sleep 3; printf '\e]9;4;0\a'
```

## Receiving Claude Code notifications

By default Claude Code sends desktop notifications only when it detects Ghostty, Kitty, or iTerm2. sshc is not part of that detection, so set the channel explicitly in `~/.claude/settings.json`. `iterm2` sends OSC 9 and `kitty` sends OSC 99; sshc accepts both.

```json
{
  "preferredNotifChannel": "iterm2"
}
```

Task completion and permission prompts then arrive as notifications. When Claude Code sends its task summary as the terminal title, the pane name follows it as well.

## Receiving Codex notifications

In Codex's `~/.codex/config.toml`, enable TUI notifications and pin the method to `osc9`. With `auto`, Codex may not recognise sshc as OSC 9-capable and fall back to BEL.

```toml
[tui]
notifications = true
notification_method = "osc9"
```

`notifications` also accepts a list such as `["agent-turn-complete", "approval-requested"]` to limit the kinds of events.

## Inside tmux

To let titles and notifications from programs inside tmux reach the outer terminal, add the following to `~/.tmux.conf`:

```text
set -g allow-passthrough on
set -g set-titles on
```

## If nothing changes

1. Confirm that `sshc --version` reports v0.35.0 or later.
2. Run the `printf` examples above inside a pane. If the pane name or unread mark changes, sshc itself is working.
3. Start a new Claude Code or Codex session after saving its settings.
4. When the agent runs on an SSH host, the settings belong in the home directory of that host.
5. When tmux or screen is in between, check its passthrough settings.

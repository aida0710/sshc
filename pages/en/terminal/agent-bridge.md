---
title: Coding Agent integration
description: Install sshc-agent-bridge to show, notify, and resume Claude Code, Codex, and OpenCode sessions.
---

# Coding Agent integration

[sshc-agent-bridge](https://github.com/aida0710/sshc-agent-bridge) is an optional plugin that reports lifecycle events from Claude Code, Codex, and OpenCode to the sshc terminal pane running the agent. Pane headers can show when an agent is working, needs input, or has finished, along with the session name when available.

::: warning Experimental feature
Agent Bridge and its protocol are still experimental. After an update, review each agent's hooks and verify state reporting in a new session.
:::

You do not need the bridge for ordinary terminal or CLI use. Without it, sshc does not infer agent state from shell output.

## Requirements

- sshc v0.19.0 or later
- Linux or macOS; a PTY inside WSL is treated as a Unix environment
- For Codex and Claude Code, Python 3.10 or later must be available as `python3` on the agent's `PATH`
- For OpenCode, Node.js 18 or later must be available

Native Windows sessions are not supported. If the agent runs on an SSH host or in a development container, install the bridge in that environment. Installing it only on the local machine does not run hooks on the remote host.

See [GitHub Releases](https://github.com/aida0710/sshc-agent-bridge/releases/latest) for the current bridge release.

## Install for Codex

Run these commands in a shell:

```sh
codex plugin marketplace add aida0710/sshc-agent-bridge
codex plugin add sshc-agent-bridge@sshc
```

Start a new Codex session, open `/hooks`, and review and trust the plugin hooks. An update that changes a hook's hash requires another review before Codex runs it again.

## Install for Claude Code

Run these slash commands inside Claude Code:

```text
/plugin marketplace add aida0710/sshc-agent-bridge
/plugin install sshc-agent-bridge@sshc
```

Enable the plugin, then start a new Claude Code session.

## Install for OpenCode

The OpenCode adapter is not published to npm yet. Clone the repository and add the absolute path to `opencode/index.js` to the `plugin` array in `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["file:///absolute/path/to/sshc-agent-bridge/opencode/index.js"]
}
```

## States shown in the terminal

| Label | Meaning |
|---|---|
| working | The agent is generating a response |
| input needed | The agent is waiting for permission or user input |
| ready | The response has finished and the agent is waiting for another prompt |
| state unavailable | A transient update expired, or a resumable candidate remains after the process exited |

When a session name is available, sshc also uses it for the pane title. Codex resolves it from the local session index, Claude Code from that session's transcript, and OpenCode from official title events. A title may not exist at session startup; until a later hook reports one, sshc retains the pane title or connection alias.

## Configure notifications

Under **Settings → Notifications**, enable browser notifications for input-needed and completion events while the sshc tab is in the background. The same screen provides separate sound choices for input-needed and completion events, a volume control, and a no-sound option. Notification permission and sound preferences are stored per browser.

## Resume an agent session

After an agent process exits with a native session ID reported by the bridge, an SSH pane offers **Resume here** and **Resume in new pane**. sshc does not resume a session until you explicitly choose one of them.

The resume action uses the original connection alias, verifies that it still resolves to the same SSH identity, and runs the fixed resume command for that agent. sshc refuses the action if the destination identity or the observed candidate has changed. Local shells can display state, but sshc does not offer session resume for them.

## Data handled by the bridge

The bridge sends only:

- Agent kind and lifecycle state
- Native session ID
- Working directory
- Model ID when supplied by an official hook
- Session name when one can be resolved

It does not send prompts, responses, tool input or output, credentials, transcript paths, or transcript contents. To resolve a session name, it reads at most the last 1 MiB of the Codex session index or that Claude Code session's transcript. Only the matching session name is included in the terminal event.

The bridge makes no network requests, rewrites no configuration files, and does not change agent permission decisions. It writes the state as an OSC sequence to the terminal device running the agent. See the bridge repository's [Security policy](https://github.com/aida0710/sshc-agent-bridge/blob/main/SECURITY.md) for its threat model and data-handling details.

## If no state appears

1. Confirm that `sshc --version` reports v0.19.0 or later.
2. Start a new agent session after installing the bridge.
3. For Codex, open `/hooks` and confirm that the hooks are trusted.
4. For Codex or Claude Code, run `python3 --version` in the environment running the agent.
5. If the agent runs over SSH or in a container, confirm that the bridge is installed there.
6. Update to the latest bridge. Terminal resolution for Claude Code was fixed in v0.1.3.

If a hook cannot find a terminal or receives an unknown event, it exits without interrupting the agent. The agent may therefore continue normally even when no state is visible in sshc.

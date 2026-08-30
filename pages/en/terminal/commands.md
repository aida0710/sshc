---
title: Quick Commands and Snippets
description: Save, preview, and send commands to one pane or a workspace.
---

# Quick Commands and Snippets

Open Quick Commands from the terminal overflow menu to insert, run, or copy a saved Snippet in the current pane.

![Terminal Quick Commands menu](/images/terminal-actions.png)

Create named commands and variables under Menu → Snippets. Secret variables are handled separately and the library is encrypted with the vault master key.

Execution has a preview step showing targets, expanded commands, and required inputs. If the terminal process changes after preview, sshc refuses to send to the replacement process.

With two or more panes, Command Center can target selected SSH and local-shell panes. It previews an ad-hoc command or Snippet before writing the command and Enter to each PTY.

::: warning Secrets
The normal preview replaces secret variables with `[secret]`. After confirmation, sshc writes the expanded value to the PTY. It may remain in remote shell history, TTY echo, or scrollback, so verify the targets and command before sending it.
:::

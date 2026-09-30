---
title: Quick Commands and Snippets
description: Save, preview, and send commands to one pane or a workspace.
---

# Quick Commands and Snippets

Open Quick Commands from the terminal overflow menu to insert, run, or copy a saved Snippet in the current pane.

![Terminal Quick Commands menu](/images/terminal-actions.png)

Create named commands and variables under Menu → Snippets. Secret variables are handled separately and the library is encrypted with the vault master key.

Execution has a preview step showing targets, expanded commands, and required inputs. If the terminal process changes after preview, sshc refuses to send to the replacement process.

Under Menu → Snippets, "Connection startup" assigns one Snippet per host. On the first connection and on every automatic reconnection, sshc sends the command and Enter only after authentication and the remote shell have started; it never writes to an authentication prompt. Variable values, secret ones included, are stored encrypted with the assignment, and because the expanded value is typed on every connection it may remain in the remote shell history, TTY echo, or scrollback.

An assignment is bound to the destination and authentication settings it resolved to at that time: host name, user, port, jump hosts, ProxyCommand, ForwardAgent, the VPN profile attached to the connection, and so on. If any of these change after the assignment, sshc stops sending the Snippet to that host so that secret values do not reach a different machine. The terminal says that the Snippet was not sent, and "Connection startup" on the Snippets screen lists the hosts whose assignment stopped. Check the destination and assign it again.

For the VPN profile, the assignment stops in these cases, because the same address can be a different machine on the network of another VPN:

- A VPN profile is attached to the connection, detached from it, or replaced with another profile.
- The VPN profile attached to the connection is deleted. Attaching a profile recreated under the same name does not bring the assignment back, even when the profile was detached from the connection before it was deleted; assign it again.

Renaming or editing the VPN profile keeps the assignment working.

Assignments made by an earlier version, which did not record the destination, are stopped in the same way. Saving a connection on the Connections screen so that its saved password or one-time password works on the new route does not move a startup snippet assignment.

The Snippets screen can also run one Snippet against several hosts as non-interactive SSH executions. It previews the expanded command and the resolved targets before starting and reports the result per host. One run takes up to 64 targets and executes 4 at a time. These executions use their own connections and do not inherit the working directory or shell state of an open pane.

With two or more panes, Command Center can target selected SSH and local-shell panes. It previews an ad-hoc command or Snippet before writing the command and Enter to each PTY.

::: warning Secrets
The normal preview replaces secret variables with `[secret]`. After confirmation, sshc writes the expanded value to the PTY. It may remain in remote shell history, TTY echo, or scrollback, so verify the targets and command before sending it.
:::

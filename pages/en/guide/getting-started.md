---
title: What is sshc?
description: A local application for OpenSSH configuration, connections, terminals, SFTP, workspaces, and encrypted sync.
---

# What is sshc?

sshc makes an existing OpenSSH environment easier to operate without replacing it. It builds connection management, terminals, SFTP, multi-pane workspaces, and encrypted sync around `~/.ssh/config`, `Include`, private keys, and `known_hosts`.

![Connection management](/images/connections-desktop.png)

## When to use it

- You want the same OpenSSH configuration in a CLI and a GUI.
- Your hosts, keys, jump routes, and groups no longer fit in your head.
- You want SFTP and port forwarding without leaving the terminal workflow.
- You need encrypted workspace sync across devices.
- You use the same connection set on desktop and Android.

## What it is not

sshc is not an SSH server or a cloud relay. Connections and decryption happen on your device. It does not upload terminal scrollback or running processes. OpenSSH files remain usable by the normal `ssh` command.

## At a glance

| Area | What it covers |
| --- | --- |
| Connections | Host blocks, Include trees, groups, keys, passwords, known hosts |
| Terminal | SSH/local shells, search, links, encodings, Quick Commands |
| Workspace | Up to four panes, drag splits, saved ratios, broadcast commands |
| SFTP | File browser, editor, folder transfer, resumable queue |
| Sync | S3-compatible storage, local encryption, push, pull, history |
| CLI | SSH, sync, terminal control, Serial, Telnet, JSON automation |

Next, [install sshc](/en/guide/install) and open your [first connection](/en/guide/first-connection).

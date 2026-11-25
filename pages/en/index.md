---
layout: home
title: sshc
description: A terminal app that uses your existing OpenSSH configuration, with remote sessions, Claude Code status, SFTP, reusable credentials, an AI-friendly CLI, per-connection VPN, and end-to-end encrypted (E2EE) sync.
sidebar: false
outline: false
docsHome:
  name: sshc
  lead: >-
    sshc is a terminal app that uses your existing OpenSSH configuration. It combines SSH and local shells with remote sessions, Claude Code status, SFTP, reusable credentials, a CLI for AI agents, per-connection VPN, and end-to-end encrypted (E2EE) sync across devices through S3-compatible storage you provide.
  platforms:
    label: Platforms
    text: macOS / Windows / Linux / Android
  actions:
    - { text: Install, link: "/en/guide/install", primary: true }
    - { text: Get started, link: "/en/guide/getting-started" }
    - { text: GitHub, link: "https://github.com/aida0710/sshc" }
  preview:
    src: /images/terminal-desktop.png
    alt: A local shell open in a named workspace
    width: 1280
    height: 720
  sections:
    - heading: Features
      lead: >-
        Open SSH sessions and local shells side by side in one list, then use SFTP and port forwarding with the same connections. Connection settings remain in OpenSSH format. Chosen connections can also go through <a href="./features/vpn">a VPN of their own</a>.
      features:
        - title: Remote sessions
          link: /en/features/remote-session
          image: /images/remote-session-desktop-en.png
          alt: The remote session menu with the engine on an SSH host chosen
          body: >-
            Run terminals on the sshc engine of an always-on server, and use them from this machine's browser or the Android app. Claude Code and other programs keep running on the server after the browser or this machine is closed.
          wide: true
        - title: Claude Code status, automatically
          link: /en/terminal/notifications#claude-code-status
          image: /images/program-status-desktop-en.png
          alt: The session list showing needs permission, working and done
          body: >-
            Start Claude Code and the session list shows when it is working, needs permission, has a question or is done, with no settings or hooks. When it starts waiting for you, a browser or Android notification tells you too.
          wide: true
        - title: SSH and local shells
          image: /images/terminal-desktop.png
          alt: A local shell open in a named workspace
          body: >-
            Switch between sessions from the list. Reconnect, search, and forward ports.
        - title: Use OpenSSH configuration directly
          image: /images/connections-desktop.png
          alt: Connections screen for organizing OpenSSH hosts
          body: >-
            Keep <code>~/.ssh/config</code>, Include, and Match intact, so regular ssh and VS Code use the same aliases.
        - title: Reuse saved credentials
          image: /images/credentials-desktop.png
          alt: Vault screen assigning a saved password to multiple hosts
          body: >-
            Register passwords and key passphrases in the vault once, then use them from the terminal, SFTP, and CLI without configuring each feature separately.
        - title: One-time passwords
          image: /images/otp-desktop-en.png
          alt: OTP screen showing current codes and assigned hosts
          body: >-
            Store TOTP setup keys in the vault to see the current codes. Assign one to a connection, and sshc enters the code when the server asks for it.
        - title: CLI for AI agents
          image: /images/cli-desktop.png
          alt: Terminal running the sshc non-interactive CLI
          body: >-
            Codex and other agents can call sshc directly and connect with saved credentials.
        - title: SFTP
          image: /images/sftp-desktop.png
          alt: SFTP screen for working with remote files
          body: >-
            Edit remote files, transfer folders, compare two remote connections, and copy directly between them alongside the terminal.
        - title: End-to-end encrypted sync
          image: /images/sync-desktop-en.png
          alt: Sync screen for managing encrypted snapshots
          body: >-
            Encrypt connections, keys, credentials, and snippets on your device before syncing through S3-compatible storage you provide. Only devices configured with the same sync key can decrypt them; neither the storage provider nor the sshc developers can read them in plaintext.
        - title: Per-connection VPN
          image: /images/vpn-desktop-en.png
          alt: VPN screen listing profiles and the connections that use them
          body: >-
            Route only the connections you choose through a VPN of their own, without changing this machine's routing or DNS. WireGuard, OpenVPN, IKEv2/IPsec, and more are supported.
        - title: Android app
          image: /images/android-features.png
          alt: Home and Connections screens of the Android app
          body: >-
            Use the same interface on Android. Work with SSH, SFTP, and a local shell through the bottom navigation and an extra key row.
    - heading: Try the demo
      lead: >-
        Start three Linux VMs in your browser and try SSH connections and SFTP from the sshc Web UI and CLI. A desktop browser is recommended.
      actions:
        - { text: Open the demo, link: "https://sshc-demo.aida0710.work/index.html", primary: true }
    - heading: Install
      lead: >-
        Install through Homebrew on macOS and Linux, or use the verified PowerShell installer from GitHub Releases on Windows.
      command:
        code: brew install aida0710/tap/sshc
        label: macOS / Linux
---

<DocsHome />

---
layout: home
title: sshc
description: A terminal app that uses your existing OpenSSH configuration, with SFTP, reusable credentials, an AI-friendly CLI, per-connection VPN, and encrypted sync.
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <h1 class="sshc-title">sshc</h1>
      <p class="sshc-lead">sshc is a terminal app that uses your existing OpenSSH configuration. It combines SSH and local shells with SFTP, reusable credentials, a CLI for AI agents, per-connection VPN, and encrypted sync across devices through S3-compatible storage you provide.</p>
      <p class="sshc-platforms"><span>Platforms</span>macOS / Windows / Linux / Android</p>
      <div class="sshc-actions">
        <a class="sshc-action primary" href="./guide/install">Install</a>
        <a class="sshc-action" href="./guide/getting-started">Get started</a>
        <a class="sshc-action" href="https://github.com/aida0710/sshc">GitHub</a>
      </div>
    </div>
    <div class="sshc-preview">
      <img src="/images/workspace-desktop.png" alt="Four SSH connections open in the sshc terminal" width="1280" height="720">
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>Features</h2>
      <p>Open SSH sessions and local shells in multiple panes, then use SFTP and port forwarding with the same connections. Connection settings remain in OpenSSH format. Chosen connections can also go through <a href="./features/vpn">a VPN of their own</a>.</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/workspace-desktop.png" alt="A Workspace with multiple SSH connections open" width="1280" height="720"><div class="sshc-feature-body"><span class="index">01</span><h3>SSH and local shells</h3><p>Reconnect, search, forward ports, and arrange up to four panes in one terminal.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/connections-desktop.png" alt="Connections screen for organizing OpenSSH hosts" width="1280" height="720"><div class="sshc-feature-body"><span class="index">02</span><h3>Use OpenSSH configuration directly</h3><p>Keep <code>~/.ssh/config</code>, Include, and Match intact, so regular ssh and VS Code use the same aliases.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/credentials-desktop.png" alt="Vault screen assigning a saved password to multiple hosts" width="1280" height="720"><div class="sshc-feature-body"><span class="index">03</span><h3>Reuse saved credentials</h3><p>Register passwords and key passphrases in the vault once, then use them from the terminal, SFTP, and CLI without configuring each feature separately.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/otp-desktop-en.png" alt="OTP screen showing current codes and assigned hosts" width="1280" height="720"><div class="sshc-feature-body"><span class="index">04</span><h3>One-time passwords</h3><p>Store TOTP setup keys in the vault to see the current codes. Assign one to a connection, and sshc enters the code when the server asks for it.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/cli-desktop.png" alt="Terminal running the sshc non-interactive CLI" width="1280" height="720"><div class="sshc-feature-body"><span class="index">05</span><h3>CLI for AI agents</h3><p>Codex and other agents can call sshc directly and connect with saved credentials.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sftp-desktop.png" alt="SFTP screen for working with remote files" width="1280" height="720"><div class="sshc-feature-body"><span class="index">06</span><h3>SFTP</h3><p>Edit remote files, transfer folders, compare two remote connections, and copy directly between them alongside the terminal.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sync-desktop-en.png" alt="Sync screen for managing encrypted snapshots" width="1280" height="720"><div class="sshc-feature-body"><span class="index">07</span><h3>Encrypted sync</h3><p>Encrypt connections, keys, credentials, and snippets before syncing through S3-compatible storage you provide. sshc does not host or retain the synced data.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/vpn-desktop-en.png" alt="VPN screen listing profiles and the connections that use them" width="1280" height="720"><div class="sshc-feature-body"><span class="index">08</span><h3>Per-connection VPN</h3><p>Route only the connections you choose through a VPN of their own, without changing this machine's routing or DNS. WireGuard, OpenVPN, IKEv2/IPsec, and more are supported.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/android-features.png" alt="Home and Connections screens of the Android app" width="1280" height="720"><div class="sshc-feature-body"><span class="index">09</span><h3>Android app</h3><p>Use the same interface on Android. Work with SSH, SFTP, and a local shell through the bottom navigation and an extra key row.</p></div></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>Try the demo</h2>
      <p>Start three Linux VMs in your browser and try SSH connections and SFTP from the sshc Web UI and CLI. A desktop browser is recommended.</p>
    </div>
    <div class="sshc-actions"><a class="sshc-action primary" href="https://sshc-demo.aida0710.work/index.html">Open the demo</a></div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>Install</h2>
      <p>Install through Homebrew on macOS and Linux, or use the verified PowerShell installer from GitHub Releases on Windows.</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

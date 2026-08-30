---
layout: home
title: sshc
description: A terminal app that uses your existing OpenSSH configuration, with SFTP, reusable credentials, an AI-friendly CLI, and encrypted sync.
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <h1 class="sshc-title">sshc</h1>
      <p class="sshc-lead">sshc is a terminal app that uses your existing OpenSSH configuration. It combines SSH and local shells with SFTP, reusable credentials, a CLI for AI agents, and encrypted sync across devices through S3-compatible storage you provide.</p>
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
      <p>Open SSH sessions and local shells in multiple panes, then use SFTP and port forwarding with the same connections. Connection settings remain in OpenSSH format.</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/workspace-desktop.png" alt="A Workspace with four SSH connections open" width="1280" height="720"><div class="sshc-feature-body"><span class="index">01</span><h3>SSH and local shells</h3><p>Reconnect, search, forward ports, and arrange up to four panes in one terminal.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/connections-desktop.png" alt="Connections screen for organizing OpenSSH hosts" width="1280" height="720"><div class="sshc-feature-body"><span class="index">02</span><h3>Use OpenSSH configuration directly</h3><p>Keep <code>~/.ssh/config</code>, Include, and Match intact, so regular ssh and VS Code use the same aliases.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/credentials-desktop.png" alt="Vault screen assigning a saved password to multiple hosts" width="1280" height="720"><div class="sshc-feature-body"><span class="index">03</span><h3>Reuse saved credentials</h3><p>Register passwords and key passphrases in the vault once, then use them from the terminal, SFTP, and CLI without configuring each feature separately.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/cli-desktop.png" alt="Terminal running the sshc non-interactive CLI" width="1280" height="720"><div class="sshc-feature-body"><span class="index">04</span><h3>CLI for AI agents</h3><p>Codex and other agents can call sshc directly and connect with saved credentials.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sftp-desktop.png" alt="SFTP screen for working with remote files" width="1280" height="720"><div class="sshc-feature-body"><span class="index">05</span><h3>SFTP</h3><p>Edit remote files and transfer folders alongside an active terminal. File transfers resume when their recovery requirements are met.</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sync-desktop.png" alt="Sync screen for managing encrypted snapshots" width="1280" height="720"><div class="sshc-feature-body"><span class="index">06</span><h3>Encrypted sync</h3><p>Encrypt connections, keys, credentials, and snippets before syncing through S3-compatible storage you provide. sshc does not host or retain the synced data.</p></div></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>Install</h2>
      <p>Install through Homebrew on macOS and Linux, or use the verified PowerShell installer from GitHub Releases on Windows.</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

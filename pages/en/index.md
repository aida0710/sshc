---
layout: home
title: sshc
description: Organize OpenSSH, reuse saved credentials from the CLI, and sync securely across devices.
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <h1 class="sshc-title">sshc</h1>
      <p class="sshc-lead">Organize your existing <code>~/.ssh/config</code>, sync it across devices, and reuse saved credentials from the terminal, SFTP, or CLI.</p>
      <div class="sshc-actions">
        <a class="sshc-action primary" href="./guide/install">Install</a>
        <a class="sshc-action" href="./guide/getting-started">Get started</a>
        <a class="sshc-action" href="https://github.com/aida0710/sshc">GitHub</a>
      </div>
    </div>
    <div class="sshc-preview">
      <img src="/images/connections-desktop.png" alt="The sshc connection manager" width="1600" height="1000">
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>OpenSSH configuration and credentials</h2>
      <p>Your OpenSSH files remain the source of truth, so VS Code, Codex, and regular ssh commands use the same aliases. AI agents can call the sshc CLI while sshc supplies credentials from the unlocked vault.</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><span class="index">01</span><h3>Organize OpenSSH</h3><p>Keep <code>~/.ssh/config</code>, Include, and Match intact while organizing connections and groups.</p></article>
      <article class="sshc-feature"><span class="index">02</span><h3>Save credentials once</h3><p>Store passwords and key passphrases in the vault, then reuse them from the terminal, SFTP, and CLI.</p></article>
      <article class="sshc-feature"><span class="index">03</span><h3>CLI for AI agents</h3><p>Codex and other agents can call sshc directly. With the vault unlocked, sshc supplies the saved credentials.</p></article>
      <article class="sshc-feature"><span class="index">04</span><h3>Encrypted sync</h3><p>Encrypt connections, keys, credentials and snippets before syncing through S3-compatible storage.</p></article>
      <article class="sshc-feature"><span class="index">05</span><h3>Terminal and SFTP</h3><p>Reconnect, forward ports, split panes, and transfer files from one local application.</p></article>
      <article class="sshc-feature"><span class="index">06</span><h3>Local-first</h3><p>No mandatory account or hosted control plane. Credentials are encrypted on the device you control.</p></article>
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

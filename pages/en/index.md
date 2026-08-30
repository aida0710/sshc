---
layout: home
title: sshc — Manage OpenSSH without replacing it
description: Connections, terminals, SFTP, workspaces and encrypted sync in one local application.
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <p class="sshc-eyebrow">Local-first SSH workspace</p>
      <h1 class="sshc-title">Manage OpenSSH.<br>Keep OpenSSH.</h1>
      <p class="sshc-lead">Use your existing <code>~/.ssh/config</code> for connections, terminals, SFTP, workspaces and encrypted sync—without trapping it in a proprietary format.</p>
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
      <h2>Keep the source.<br>Add better workflows.</h2>
      <p>sshc preserves comments, ordering and Include directives. Close the browser and your source of truth is still a set of files OpenSSH can read.</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><span class="index">01</span><h3>Connections and terminal</h3><p>Manage ProxyJump, encodings, search, reconnect and Local or SOCKS forwarding per connection.</p></article>
      <article class="sshc-feature"><span class="index">02</span><h3>SFTP</h3><p>Transfer files and folders with resume, progress and conflict handling without leaving the terminal workflow.</p></article>
      <article class="sshc-feature"><span class="index">03</span><h3>Workspaces</h3><p>Split up to four sessions, save the arrangement and reopen it later as fresh sessions.</p></article>
      <article class="sshc-feature"><span class="index">04</span><h3>Encrypted sync</h3><p>Encrypt connections, keys, credentials and snippets before syncing through S3-compatible storage.</p></article>
      <article class="sshc-feature"><span class="index">05</span><h3>CLI and Web UI</h3><p>Use one running engine for interactive SSH, status, sync and terminal automation from the CLI.</p></article>
      <article class="sshc-feature"><span class="index">06</span><h3>Local first</h3><p>No mandatory account or hosted control plane. Credentials are encrypted on the device you control.</p></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>Start with one command.</h2>
      <p>Install through Homebrew on macOS and Linux, or use the verified PowerShell installer from GitHub Releases on Windows.</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

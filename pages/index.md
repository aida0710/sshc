---
layout: home
title: sshc
description: SSHとローカルシェルを扱うターミナルアプリ。OpenSSH設定をそのまま使い、SFTP、認証情報の再利用、AIエージェント向けCLI、暗号化同期に対応。
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <h1 class="sshc-title">sshc</h1>
      <p class="sshc-lead">sshcは、SSHとローカルシェルを扱うターミナルアプリです。今あるOpenSSHの設定をそのまま使い、SFTP、認証情報の再利用、AIエージェント向けCLI、端末間の暗号化同期に対応します。</p>
      <div class="sshc-actions">
        <a class="sshc-action primary" href="./guide/install">インストール</a>
        <a class="sshc-action" href="./guide/getting-started">はじめる</a>
        <a class="sshc-action" href="https://github.com/aida0710/sshc">GitHub</a>
      </div>
    </div>
    <div class="sshc-preview">
      <img src="/images/terminal-desktop.png" alt="sshcでSSH接続を開いたTerminal画面" width="1280" height="720">
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>主な機能</h2>
      <p>SSHとローカルシェルを複数ペインで開き、SFTPやポート転送も同じ接続先から利用できます。接続設定はOpenSSH形式のまま管理します。</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><span class="index">01</span><h3>SSHとローカルシェル</h3><p>再接続、検索、ポート転送、最大4ペインのWorkspaceを一つのTerminalで利用できます。</p></article>
      <article class="sshc-feature"><span class="index">02</span><h3>OpenSSH設定をそのまま使用</h3><p><code>~/.ssh/config</code>、<code>Include</code>、<code>Match</code>を保つため、通常のsshやVS Codeも同じエイリアスを使えます。</p></article>
      <article class="sshc-feature"><span class="index">03</span><h3>認証情報は一度保存</h3><p>パスワードや鍵のパスフレーズをVaultに保存し、Terminal、SFTP、CLIから再利用できます。</p></article>
      <article class="sshc-feature"><span class="index">04</span><h3>AIからCLIで操作</h3><p>CodexなどのAIエージェントがsshcを直接実行し、保存済みの認証情報で接続できます。</p></article>
      <article class="sshc-feature"><span class="index">05</span><h3>SFTP</h3><p>リモートファイルの編集、フォルダー転送、中断と再開を、接続中のTerminalと並行して行えます。</p></article>
      <article class="sshc-feature"><span class="index">06</span><h3>暗号化同期</h3><p>接続設定、鍵、資格情報、スニペットを端末上で暗号化し、S3互換ストレージを介して同期します。</p></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>インストール</h2>
      <p>macOSとLinuxではHomebrewから。Windowsには、GitHub Releasesで配布する検証済みのPowerShellインストーラーがあります。</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

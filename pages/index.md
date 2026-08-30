---
layout: home
title: sshc — OpenSSHを、そのまま使いやすく
description: OpenSSH設定の整理、認証情報の再利用、AIエージェント向けCLI、暗号化同期を一つに。
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <p class="sshc-eyebrow">Local-first SSH workspace</p>
      <h1 class="sshc-title">OpenSSHを、<br>そのまま使いやすく。</h1>
      <p class="sshc-lead">いつもの <code>~/.ssh/config</code> をそのまま整理し、端末間で同期。保存した認証情報は、Terminal、SFTP、CLIから繰り返し使えます。</p>
      <div class="sshc-actions">
        <a class="sshc-action primary" href="./guide/install">インストール</a>
        <a class="sshc-action" href="./guide/getting-started">はじめる</a>
        <a class="sshc-action" href="https://github.com/aida0710/sshc">GitHub</a>
      </div>
    </div>
    <div class="sshc-preview">
      <img src="/images/connections-desktop.png" alt="sshcの接続管理画面" width="1600" height="1000">
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>一度整えれば、<br>人にもAIにも使いやすい。</h2>
      <p>設定の正本は、OpenSSHが読めるファイルのまま。VS CodeやCodexは同じエイリアスを使えます。AIエージェントからはsshcのCLIを呼び出し、保存済みの認証情報で接続できます。</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><span class="index">01</span><h3>OpenSSHのまま整理</h3><p><code>~/.ssh/config</code>、<code>Include</code>、<code>Match</code>を保ったまま、接続先やグループを見やすく整理します。</p></article>
      <article class="sshc-feature"><span class="index">02</span><h3>認証情報は一度保存</h3><p>パスワードや鍵のパスフレーズをVaultに保存。Terminal、SFTP、CLIで再入力する手間を減らします。</p></article>
      <article class="sshc-feature"><span class="index">03</span><h3>AIからCLIで操作</h3><p>CodexなどのAIエージェントがsshcを直接実行。Vaultが開いていれば、保存済みの認証情報をsshcが使います。</p></article>
      <article class="sshc-feature"><span class="index">04</span><h3>暗号化同期</h3><p>接続設定、鍵、資格情報、スニペットを端末上で暗号化し、S3互換ストレージを介して同期します。</p></article>
      <article class="sshc-feature"><span class="index">05</span><h3>TerminalとSFTP</h3><p>再接続、ポート転送、複数ペイン、ファイル転送。普段のSSH作業を一つの画面にまとめます。</p></article>
      <article class="sshc-feature"><span class="index">06</span><h3>ローカル優先</h3><p>アカウント登録も、専用クラウドも不要です。資格情報は手元の端末で暗号化します。</p></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>まずは、一行から。</h2>
      <p>macOSとLinuxではHomebrewから。Windowsには、GitHub Releasesで配布する検証済みのPowerShellインストーラーがあります。</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

---
layout: home
title: sshc — OpenSSHを、そのまま管理する
description: 接続、Terminal、SFTP、Workspace、暗号化同期を一つのローカルアプリケーションで。
sidebar: false
outline: false
---

<main class="sshc-home">
  <section class="sshc-hero">
    <div>
      <p class="sshc-eyebrow">Local-first SSH workspace</p>
      <h1 class="sshc-title">OpenSSHを、<br>そのまま管理する。</h1>
      <p class="sshc-lead">既存の <code>~/.ssh/config</code> を独自形式へ閉じ込めず、接続、Terminal、SFTP、Workspace、暗号化同期を一つのローカルアプリケーションで扱います。</p>
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
      <h2>設定を置き換えない。<br>使う場所を増やす。</h2>
      <p>コメント、記述順、Includeを保ったまま編集します。ブラウザを閉じても、設定の正本はOpenSSHが読めるファイルのままです。</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><span class="index">01</span><h3>接続とTerminal</h3><p>ProxyJump、文字コード、検索、再接続、Local／SOCKSポート転送を接続ごとに管理します。</p></article>
      <article class="sshc-feature"><span class="index">02</span><h3>SFTP</h3><p>ファイルとフォルダの転送、再開、進捗、競合をTerminalから離れずに扱います。</p></article>
      <article class="sshc-feature"><span class="index">03</span><h3>Workspace</h3><p>最大4つの接続を分割し、配置を保存して、必要なときに新しいセッションとして開き直します。</p></article>
      <article class="sshc-feature"><span class="index">04</span><h3>暗号化同期</h3><p>接続設定、鍵、資格情報、Snippetを暗号化し、S3互換ストレージ経由で同期します。</p></article>
      <article class="sshc-feature"><span class="index">05</span><h3>CLIとWeb UI</h3><p>同じengineを正本として、対話接続、状態確認、同期、Terminal操作をCLIから自動化できます。</p></article>
      <article class="sshc-feature"><span class="index">06</span><h3>ローカル優先</h3><p>アカウント登録や外部サービスを必須にせず、資格情報は利用者の端末内で暗号化します。</p></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>導入は一行から。</h2>
      <p>macOSとLinuxではHomebrew、WindowsではGitHub Releaseの検証済みPowerShell installerを利用できます。</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

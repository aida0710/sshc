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
      <p class="sshc-lead">sshcは、SSHとローカルシェルを扱うターミナルアプリです。<br>今あるOpenSSH設定をそのまま使えます。SFTP、認証情報の再利用、AIエージェント向けCLIに加え、利用者が用意したS3互換ストレージを介した暗号化同期にも対応しています。</p>
      <p class="sshc-platforms"><span>対応OS</span>macOS / Windows / Linux / Android</p>
      <div class="sshc-actions">
        <a class="sshc-action primary" href="./guide/install">インストール</a>
        <a class="sshc-action" href="./guide/getting-started">はじめる</a>
        <a class="sshc-action" href="https://github.com/aida0710/sshc">GitHub</a>
      </div>
    </div>
    <div class="sshc-preview">
      <img src="/images/workspace-desktop.png" alt="sshcで複数のSSH接続を開いたTerminal画面" width="1280" height="720">
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>主な機能</h2>
      <p>SSHとローカルシェルを複数のペインで開けます。SFTPやポート転送にも同じ接続設定を使い、OpenSSH形式のまま管理できます。</p>
    </div>
    <div class="sshc-feature-grid">
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/workspace-desktop.png" alt="複数のSSH接続を開いたワークスペース" width="1280" height="720"><div class="sshc-feature-body"><span class="index">01</span><h3>SSHとローカルシェル</h3><p>1つのTerminalを最大4ペインに分割できます。再接続、検索、ポート転送にも対応しています。</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/connections-desktop.png" alt="OpenSSH接続を整理するConnections画面" width="1280" height="720"><div class="sshc-feature-body"><span class="index">02</span><h3>OpenSSH設定をそのまま使う</h3><p><code>~/.ssh/config</code>の<code>Include</code>や<code>Match</code>をそのまま扱っています。通常の<code>ssh</code>やVS Codeでも、同じエイリアスを使えます。</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/credentials-desktop.png" alt="保存済みパスワードを複数ホストへ割り当てたVault画面" width="1280" height="720"><div class="sshc-feature-body"><span class="index">03</span><h3>認証情報は一度保存</h3><p>パスワードや鍵のパスフレーズはVaultに一度保存すれば、Terminal、SFTP、CLIでそのまま使えます。機能ごとに設定し直す必要はありません。</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/cli-desktop.png" alt="sshcの非対話CLIを実行したTerminal画面" width="1280" height="720"><div class="sshc-feature-body"><span class="index">04</span><h3>AIエージェントから使えるCLI</h3><p>CodexなどのAIエージェントからsshcを直接実行できます。接続にはVaultに保存した認証情報を使っています。</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sftp-desktop.png" alt="リモートファイルを操作するSFTP画面" width="1280" height="720"><div class="sshc-feature-body"><span class="index">05</span><h3>SFTP</h3><p>リモートファイルの編集、フォルダー転送、2つの接続先の比較と直接コピーを、Terminalと行き来しながら操作できます。</p></div></article>
      <article class="sshc-feature"><img class="sshc-feature-image" src="/images/sync-desktop-ja.png" alt="暗号化スナップショットを管理するSync画面" width="1280" height="720"><div class="sshc-feature-body"><span class="index">06</span><h3>暗号化同期</h3><p>接続設定、鍵、認証情報、スニペットを端末上で暗号化し、利用者が用意したS3互換ストレージを介して同期できます。sshcは同期用ストレージを提供せず、データを預かりません。</p></div></article>
    </div>
  </section>

  <section class="sshc-home-section">
    <div class="sshc-section-heading">
      <h2>インストール</h2>
      <p>macOSとLinuxではHomebrewからインストールできます。Windows向けには、GitHub Releasesで検証済みのPowerShellインストーラーを配布しています。</p>
    </div>
    <div class="sshc-command"><code>brew install aida0710/tap/sshc</code><span>macOS / Linux</span></div>
  </section>
</main>

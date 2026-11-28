---
layout: home
title: sshc
description: SSHとローカルシェルを扱うターミナルアプリ。OpenSSH設定をそのまま使い、リモートセッション、Claude Codeの状態の表示、SFTP、認証情報の再利用、AIエージェント向けCLI、接続ごとのVPN、エンドツーエンド暗号化（E2EE）の同期に対応。
sidebar: false
outline: false
docsHome:
  name: sshc
  lead: >-
    sshcは、SSHとローカルシェルを扱うターミナルアプリです。<br>今あるOpenSSH設定をそのまま使えます。リモートセッション、Claude Codeの状態の表示、SFTP、認証情報の再利用、AIエージェント向けCLI、接続ごとのVPNに加え、ユーザーが用意したS3互換ストレージを介した、エンドツーエンド暗号化（E2EE）の同期にも対応しています。
  platforms:
    label: 対応OS
    text: macOS / Windows / Linux / Android
  actions:
    - { text: インストール, link: "/guide/install", primary: true }
    - { text: はじめる, link: "/guide/getting-started" }
    - { text: GitHub, link: "https://github.com/aida0710/sshc" }
  preview:
    src: /images/terminal-desktop.png
    alt: ワークスペース内でローカルシェルを開いたTerminal画面
    width: 1280
    height: 720
  sections:
    - heading: 主な機能
      lead: >-
        SSHとローカルシェルを同じ画面で開けます。SFTPやポート転送にも同じ接続設定を使い、OpenSSH形式のまま管理できます。選んだ接続だけを<a href="./features/vpn">専用のVPN</a>経由で接続することもできます。
      features:
        - title: リモートセッション
          link: /features/remote-session
          image: /images/remote-session-desktop-ja.png
          alt: リモートセッションのメニューで、接続先のsshcエンジンを選んだ画面
          body: >-
            常時動いているサーバーのsshcエンジンでターミナルを動かし、このマシンのブラウザやAndroidアプリから操作できます。ブラウザやこのマシンを閉じても、Claude Codeなどのプログラムはサーバーで動き続けます。
          wide: true
        - title: Claude Codeの状態を自動で表示
          link: /terminal/notifications#claude-codeの状態
          image: /images/program-status-desktop-ja.png
          alt: セッションの一覧に、許可待ち、作業中、完了を表示した画面
          body: >-
            Claude Codeを起動するだけで、作業中、許可待ち、回答待ち、完了をセッションの一覧に表示します。設定やフックは必要ありません。許可や回答を待ち始めると、ブラウザやAndroidの通知でも知らせます。
          wide: true
        - title: SSHとローカルシェル
          image: /images/terminal-desktop.png
          alt: ワークスペース内でローカルシェルを開いたTerminal
          body: >-
            セッションの一覧から切り替えて使えます。再接続、検索、ポート転送にも対応しています。
        - title: OpenSSH設定をそのまま使う
          image: /images/connections-desktop.png
          alt: OpenSSH接続を整理するConnections画面
          body: >-
            <code>~/.ssh/config</code>の<code>Include</code>や<code>Match</code>をそのまま扱っています。通常の<code>ssh</code>やVS Codeでも、同じ接続名を使えます。
        - title: 認証情報は一度保存
          image: /images/credentials-desktop.png
          alt: 保存済みパスワードを複数ホストへ割り当てたVault画面
          body: >-
            パスワードや鍵のパスフレーズはVaultに一度保存すれば、Terminal、SFTP、CLIでそのまま使えます。機能ごとに設定し直す必要はありません。
        - title: ワンタイムパスワード
          image: /images/otp-desktop-ja.png
          alt: 現在のコードと割り当て先を表示したOTP画面
          body: >-
            TOTPのセットアップキーをVaultに保存すると、現在のコードを確認できます。接続先に割り当てておけば、サーバーが求めたときにコードを自動で入力します。
        - title: AIエージェントから使えるCLI
          image: /images/cli-desktop.png
          alt: sshcの非対話CLIを実行したTerminal画面
          body: >-
            CodexなどのAIエージェントからsshcを直接実行できます。接続にはVaultに保存した認証情報を使っています。
        - title: SFTP
          image: /images/sftp-desktop.png
          alt: リモートファイルを操作するSFTP画面
          body: >-
            リモートファイルの編集、フォルダ転送、2つの接続先の比較と直接コピーを、Terminalと行き来しながら操作できます。
        - title: E2EEの暗号化同期
          image: /images/sync-desktop-ja.png
          alt: 暗号化スナップショットを管理するSync画面
          body: >-
            接続設定、認証情報、スニペット、同期対象のSSH鍵をこのマシン上で暗号化し、ユーザーが用意したS3互換ストレージを介して同期できます。復号できるのは、同じ暗号化キーを設定したマシンだけです。ストレージ事業者もsshcの開発者も、内容を平文では取得できません。
        - title: 接続ごとのVPN
          image: /images/vpn-desktop-ja.png
          alt: VPNプロファイルと使用する接続を並べたVPN画面
          body: >-
            選んだ接続だけを、その接続専用のVPN経由で接続します。このマシンのルーティングとDNSは変更しません。WireGuard、OpenVPN、IKEv2/IPsecなどに対応しています。
        - title: Androidアプリ
          image: /images/android-features.png
          alt: AndroidアプリのHome画面とConnections画面
          body: >-
            同じ画面をAndroidでも使えます。SSH、SFTP、ローカルシェルを、下部のナビゲーションと補助キーで操作できます。
    - heading: デモを試す
      lead: >-
        ブラウザの中でLinuxのVMを3台起動し、sshcのWeb UIとCLIからSSH接続やSFTPを試せます。デスクトップのブラウザでお試しください。
      actions:
        - { text: デモを開く, link: "https://sshc-demo.aida0710.work/index.html", primary: true }
    - heading: インストール
      lead: >-
        macOSとLinuxではHomebrewからインストールできます。Windows向けには、GitHub Releasesで検証済みのPowerShellインストーラーを配布しています。
      command:
        code: brew install aida0710/tap/sshc
        label: macOS / Linux
---

<DocsHome />

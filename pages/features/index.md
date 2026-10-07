---
title: 機能
description: SSHとローカルシェル、SFTP、OpenSSH接続管理、認証情報とワンタイムパスワード、接続ごとのVPN、AIエージェント向けCLI、暗号化同期、Androidアプリ。
---

# 機能

sshcは、SSHとローカルシェルを扱うターミナルアプリです。SFTP、ポート転送、複数ペインに加え、OpenSSH接続管理、認証情報とワンタイムパスワードの再利用、接続ごとのVPN、CLI、暗号化同期を備えています。インストールせずに試す場合は、[公開デモ](https://sshc-demo.aida0710.work/index.html)を使えます。

## Terminal

- SSHとローカルシェルを同じ画面で操作
- 接続の進行状況を表示し、終了したSSHセッションへ、ペインとスクロールバックを残したまま再接続
- 検索、文字コード、リンクやリモートパスを開く操作
- [Workspace](./workspace)で最大4ペインを配置し、レイアウトの保存と一括入力を管理
- [クイックコマンドとスニペット](/terminal/commands)を、実行前の確認、接続時の自動実行、複数の接続先への実行に利用
- [タイトルと通知](/terminal/notifications)で、プログラムが送るペイン名や通知を表示。Claude CodeやCodexの通知にも対応
- [ポート転送](/terminal/port-forwarding)はLocal forwardingとDynamic SOCKSに対応し、保存済みの設定と一時的な転送を使い分け
- Terminalのカラーパレット、フォント、背景を[設定](/reference/settings)で変更

## 接続管理

- `~/.ssh/config`、`Include`、`Match`を解析
- コメント、記述順、空白をできるだけ崩さずに編集
- [接続とグループ](/connections/manage)で接続先を検索・整理し、作成、複製、名前変更
- ホスト、設定ファイル、スニペット、設定を`Ctrl/Cmd+K`で横断検索
- 接続設定に従ってProxyJump、鍵、保存済みの認証情報を使用
- `ProxyCommand`をローカルで実行し、その標準入出力をSSH接続に使用
- OpenSSH形式を保ち、通常の`ssh`、VS Code、Codexでも同じエイリアスを使用
- ［Diagnostics］で、保存済みの接続先や一時的に指定したホストの設定を解析し、疎通を確認
- ［History］で、SSH Configや鍵などの完了した変更を確認し、中断した書き込みの復旧やファイル単位の復元を実行

## SSH鍵とKnown Hosts

- [SSH鍵](/connections/keys)の生成、名前変更、グループ間の移動、パスフレーズの編集、公開鍵のコピー、ssh-agentへの追加
- ［Remote Keys］で、`~/.ssh`の公開鍵を1台以上の接続先の`authorized_keys`へ追加
- Known Hostsを並べ替えて確認し、未登録のホスト鍵は接続時に確認。保存済みの鍵が変わっていても自動では上書きしない

## 認証情報

パスワードと鍵のパスフレーズは[Vault](/connections/credentials)に暗号化して保存し、接続先や鍵に割り当てます。Vaultへ一度登録すれば、Terminal、SFTP、ProxyJump、CLIから再利用できます。Vaultは自動ロックとマスターパスワードの変更に対応しています。

## ワンタイムパスワード（TOTP）

- Base32のセットアップキー、または`otpauth://totp/...` URIを名前付きでVaultに保存
- 現在の6桁コードを表示し、必要なときはひとつ前とひとつ後のコードも確認
- 接続先に割り当てると、サーバーが`Verification code`などを明示して求めたときだけコードを自動入力。Terminal、SFTP、`sshc ssh`、ProxyJumpの踏み台で共通
- CLIでは`sshc otp list|show|add|edit|remove`で管理

## VPN

[接続ごとのVPN](./vpn)では、選んだSSH接続だけを、その接続専用のVPN経由で接続します。このマシンのルーティングとDNSは変更しません。WireGuard、L2TP/IPsec、OpenConnect、OpenVPN、IKEv2/IPsecに対応し、Dockerが動いているマシンで使用できます。VPNのシークレットはVaultに保存します。

## SFTP

- [SFTP](./sftp)でリモートファイルを閲覧し、テキストファイルを編集
- ローカル側でも、ファイルやフォルダの削除、名前変更、フォルダ作成。削除は確認してから実行
- リモート側で、シンボリックリンクの作成とリンク先の変更、所有者とグループの変更、空き容量の表示
- 複数選択した項目の権限を、ファイルとフォルダで別々にまとめて変更
- 名前の検索と、複数のテキストファイルの内容検索
- デスクトップでは2つの接続先を並べてディレクトリを比較し、Remote→Remoteでコピー／移動。比較はサイズと更新日時のほか、SHA-256で内容まで確認可能
- [転送マネージャー](/sftp/transfers)で進行状況を確認し、一時停止・再開・キャンセル。すべての転送を合算した速度上限と、通信が切れたときの自動復旧を設定可能
- 転送キューはsshcエンジンが管理・保存するため、別画面への移動やsshcエンジン再起動後も状態を復元

## CLI

- Web UIとCLIは、共通のsshcエンジン、OpenSSH設定、Vaultを使用
- CodexなどのAIエージェントは`sshc ssh <alias> --non-interactive -- <command...>`を直接実行できる。Vaultのロックが解除されていれば、保存済みのパスワードや鍵のパスフレーズで認証
- [SerialとTelnet](/cli/serial-telnet)も、対話操作と自動処理の両方で使用可能
- `sshc update`で更新し、`sshc service install`でsshcエンジンをサービスとして登録。`sshc service restart`で再起動

## 暗号化同期

[暗号化同期](./sync)では、接続設定、認証情報、スニペット、同期対象から除外していないSSH鍵をこのマシン上で暗号化し、利用者が用意したS3互換ストレージへPush／Pullできます。ストレージ事業者は、これらの内容を平文では取得できません。バケット名やS3オブジェクト名など、暗号化されない情報は[セキュリティ](/reference/security)で確認できます。

## Android

[Androidアプリ](/platform/android)でも同じ画面を使い、SSH、SFTP、ローカルシェルを操作できます。スマートフォンでは下部のナビゲーションと補助キーで操作します。署名済みのAPKは[GitHub Releases](https://github.com/aida0710/sshc/releases)から入手できます。

## 更新と設定

- macOSとLinuxの`install.sh`で導入した場合は、新しい安定版をWeb画面から更新可能。Homebrew版は`sshc update`を使用
- アプリのテーマ（システム／ライト／ダーク）、表示言語（日本語／English）、キーボードショートカット、Vaultの自動ロックを[設定](/reference/settings)で変更

## 公開デモ

[公開デモ](https://sshc-demo.aida0710.work/index.html)では、ブラウザの中でLinuxのVMを3台起動し、インストールせずにWeb UIとCLIからSSH接続やSFTPを試せます。

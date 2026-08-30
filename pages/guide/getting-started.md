---
title: sshcとは
description: OpenSSH設定を整理し、認証情報をCLIやSFTPで再利用しながら、端末間で安全に同期する。
---

# sshcとは

sshcは、今あるOpenSSH環境を生かしたまま、接続設定と認証情報を使いやすくまとめるローカルアプリケーションです。`~/.ssh/config`、`Include`、秘密鍵、`known_hosts`を正本として、Terminal、SFTP、CLI、暗号化同期から同じ接続先を利用できます。

![接続管理画面](/images/connections-desktop.png)

## こんなときに向いています

- OpenSSHの設定を整理しながら、通常の`ssh`やVS Codeからも使い続けたい
- CodexなどのAIエージェントから、保存済みの接続先をCLIで直接使いたい
- 一度保存したパスワードや鍵のパスフレーズを、接続のたびに入力したくない
- 接続設定、鍵、資格情報を複数の端末で安全に同期したい
- Terminalを離れずにSFTP、ポート転送、複数ペインを使いたい

## 一度保存した認証情報を使い回す

接続先に割り当てたパスワードと、秘密鍵に保存したパスフレーズはVaultで暗号化されます。Vaultのロックが解除されていれば、Terminal、SFTP、ProxyJump、CLIが必要な認証情報を接続時に解決します。同じ値を機能ごとに設定し直す必要はありません。

AIエージェントも、次のようにsshcの非対話CLIを直接呼び出せます。

```sh
sshc ssh <alias> --non-interactive -- <command...>
```

AIエージェントへパスワードを渡すのではなく、sshcがVaultの保存値を認証に使います。未知のホスト鍵、2FA、Vaultのロックなど、利用者の判断や操作が必要な場合は非対話接続を続行しません。

## sshcがしないこと

sshcはSSHサーバーでも、クラウド中継サービスでもありません。接続と復号は手元の端末で行い、Terminalのスクロールバックや実行中のプロセスをクラウドへ保存しません。OpenSSH設定は独自形式へ変換しないため、VS Code、Codex、通常の`ssh`コマンドからも同じ設定を利用できます。

## 全体像

| 領域 | できること |
| --- | --- |
| Connections | `Host`設定、Include階層、グループ、鍵、保存済みパスワード、known hosts |
| Terminal | SSH／ローカルシェル、検索、リンク、文字コード、クイックコマンド |
| Workspace | 最大4ペイン、ドラッグによる分割、比率の保存、一括コマンド |
| SFTP | ファイルブラウザー、エディター、フォルダー転送、再開可能なキュー |
| Sync | S3互換ストレージ、端末内暗号化、Push／Pull／履歴 |
| CLI | 保存済み認証情報を使うSSH、同期、Terminal操作、自動化用JSON |

## 次に読む

まずは[インストール](/guide/install)へ進み、続けて[最初の接続](/guide/first-connection)を開いてください。`~/.ssh/config`がすでにあれば、起動直後から既存の接続先を利用できます。

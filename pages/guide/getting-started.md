---
title: sshcとは
description: OpenSSHの設定を中心に、接続、Terminal、SFTP、Workspace、暗号化同期をまとめるローカルアプリケーション。
---

# sshcとは

sshcは、既存のOpenSSH環境を置き換えずに扱いやすくするローカルアプリケーションです。`~/.ssh/config`、`Include`、秘密鍵、`known_hosts`を中心に、接続管理、Terminal、SFTP、複数paneのWorkspace、暗号化同期を一つの画面へまとめます。

![接続管理画面](/images/connections-desktop.png)

## こんなときに向いています

- OpenSSHの設定をCLIとGUIの両方から使いたい
- 接続先が増え、`Host`、鍵、踏み台、グループをまとめて確認したい
- Terminalを離れずにSFTPやport forwardingを使いたい
- 複数端末でSSH workspaceを安全に同期したい
- デスクトップとAndroidで同じ接続情報を使いたい

## sshcがしないこと

sshcはSSH serverでもcloud relayでもありません。接続と復号は手元の端末で行い、Terminalのscrollbackや実行中processをcloudへ保存しません。OpenSSH設定はsshc専用形式へ閉じ込めず、通常の`ssh`からも利用できる形を保ちます。

## 全体像

| 領域 | できること |
| --- | --- |
| Connections | `Host`設定、Include階層、group、鍵、password、known hosts |
| Terminal | SSH／local shell、検索、link、文字コード、Quick Commands |
| Workspace | 最大4 pane、drag分割、比率保存、一括command |
| SFTP | file browser、editor、folder転送、再開可能なqueue |
| Sync | S3互換storage、端末内暗号化、push／pull／履歴 |
| CLI | SSH、sync、Terminal操作、Serial、Telnet、自動化用JSON |

## 次に読む

まず[インストール](/guide/install)し、[最初の接続](/guide/first-connection)を開いてください。既に`~/.ssh/config`がある場合、起動直後からその接続先を利用できます。

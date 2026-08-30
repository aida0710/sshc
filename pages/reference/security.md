---
title: セキュリティ
description: sshcのlocal boundary、vault、host key、同期とTelnetの扱い。
---

# セキュリティ

## Local application

engineはloopback addressでWeb UIとAPIを提供します。UI URLは要求ごとに発行し、起動時のlogへ長期有効なURLを残しません。同じOS userとして実行できるprocessは、利用者のSSH fileへも到達できる前提です。

## Vault

account password、key passphrase、Snippetのsecretはmaster passwordで暗号化します。master passwordはcommand line引数やenvironment variableから受け取りません。12時間操作がない場合、vaultは自動lockします。

## SSH host key

未知のhost keyは利用者へ確認し、保存済みkeyの変更は拒否します。非対話SSH、SFTP、公開鍵登録では、最終hostと全ProxyJump hopが既知である必要があります。

## Sync

snapshotは端末上で専用sync keyにより暗号化してからuploadします。bucket credentialを持つ第三者は暗号文を取得し、keyの総当たりをofflineで続けられるため、十分に長いkeyを使用してください。

## Terminal data

scrollbackはmemoryだけに保持し、diskやsyncへ保存しません。OSC 52 clipboardは設定で制御できます。remote shellへ送ったsecretは、remote history、TTY echo、Terminal outputへ残る可能性があります。

## Telnet

Telnetは暗号化もserver認証もしません。sshcは警告を表示しますが、protocol自体を保護しません。資格情報を送る場合は、信頼できる隔離networkなど別の保護境界が必要です。

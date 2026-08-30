---
title: OpenSSH設定
description: ~/.ssh/config、Include、Matchを壊さずに表示・編集する。
---

# OpenSSH設定

sshcは`~/.ssh/config`を正本として読みます。`Include`階層を辿り、具体的な`Host` aliasを接続として表示します。

## Config Editor

設定file画面では、読み込まれたfileと`Include`の対応を確認し、UTF-8 textを編集できます。保存時は一時fileへ書いてから置き換えます。

UIで管理するgroup領域を除き、comment、空行、directiveの順序をできるだけ維持します。sshcが生成する範囲はmarkerで示されます。

## 接続詳細の4つのtab

- **基本**: host、user、port、認証
- **設定解析**: 解決値、出典、warning
- **詳細設定**: ProxyJump、directive、raw block
- **sshc**: 文字コード、OSC 52などsshcだけが使う設定

値を空にして保存したときはOpenSSHの継承値へ戻ります。画面上の「継承値・既定値に戻す」は、そのdirectiveを接続固有blockから削除する操作です。

## 安全に確認する

```sh
sshc info <alias>
sshc info <alias> --json
```

実接続と同じresolverで`Include`、`Match`、`ProxyJump`を解決しますが、password、passphrase、`SetEnv`の値、`ProxyCommand`本文は表示しません。

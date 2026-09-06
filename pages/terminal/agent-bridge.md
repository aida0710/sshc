---
title: Coding Agent連携
description: sshc-agent-bridgeを導入し、Claude Code、Codex、OpenCodeの状態表示、通知、セッション再開を使う。
---

# Coding Agent連携

[sshc-agent-bridge](https://github.com/aida0710/sshc-agent-bridge)は、Claude Code、Codex、OpenCodeのライフサイクルをsshcのTerminalへ伝える任意のプラグインです。エージェントを実行しているペインに、作業中、入力待ち、完了などの状態とセッション名を表示できます。

::: warning 実験的な機能
Agent Bridgeとそのプロトコルは現在も実験的です。更新後は各エージェントのフックを確認し、新しいセッションで状態表示を確かめてください。
:::

通常のTerminalやCLIを使うだけなら導入は不要です。Bridgeを入れていない場合、sshcがシェル出力からエージェントの状態を推測することもありません。

## 動作環境

- sshc v0.19.0以降
- LinuxまたはmacOS。WSL内のPTYもUnix環境として動作します
- Codex／Claude Code連携では、エージェントと同じ環境の`PATH`からPython 3.10以降を`python3`として実行できること
- OpenCode連携ではNode.js 18以降を利用できること

ネイティブWindowsのセッションには対応していません。SSH先や開発コンテナー内でエージェントを起動する場合は、そのエージェントが動く環境へBridgeを導入してください。ローカル側だけに導入しても、リモート側のフックは実行されません。

現在のBridgeリリースは[GitHub Releases](https://github.com/aida0710/sshc-agent-bridge/releases/latest)で確認できます。

## Codexへ導入する

シェルで次のコマンドを実行します。

```sh
codex plugin marketplace add aida0710/sshc-agent-bridge
codex plugin add sshc-agent-bridge@sshc
```

新しいCodexセッションを開始し、`/hooks`でプラグインフックの内容を確認して信頼してください。更新によってフックのハッシュが変わった場合は、Codexが実行を再開する前にもう一度確認を求めます。

## Claude Codeへ導入する

Claude Code内で次のスラッシュコマンドを実行します。

```text
/plugin marketplace add aida0710/sshc-agent-bridge
/plugin install sshc-agent-bridge@sshc
```

プラグインを有効にした後、新しいClaude Codeセッションを開始してください。

## OpenCodeへ導入する

OpenCode用アダプターはまだnpmで配布されていません。リポジトリをcloneし、`opencode.json`の`plugin`へ`opencode/index.js`の絶対パスを追加します。

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["file:///absolute/path/to/sshc-agent-bridge/opencode/index.js"]
}
```

## Terminalに表示される状態

| 表示 | 状態 |
|---|---|
| 作業中 | エージェントが応答を生成しています |
| 入力待ち | 権限確認や利用者の入力を待っています |
| 完了 | 応答が終わり、次の入力を待っています |
| 状態不明 | 一時的な状態の更新が途切れたか、プロセス終了後に再開候補が残っています |

セッション名を取得できた場合は、ペインのタイトルにも反映されます。Codexではローカルのセッションインデックス、Claude Codeではそのセッションのトランスクリプト、OpenCodeでは公式イベントのタイトルを使います。開始直後は名前がまだ作られていないことがあり、その場合は次のフックで更新されるまでペイン名または接続エイリアスを表示します。

## 通知を設定する

［Settings］→［Notifications］でブラウザー通知を有効にすると、sshcのタブがバックグラウンドにある間、入力待ちと完了を通知できます。同じ画面で、入力待ちと完了それぞれの通知音、音量、音なしを選べます。通知権限と通知音の設定はブラウザーごとに保存されます。

## エージェントのセッションを再開する

BridgeがネイティブセッションIDを報告したエージェントのプロセスが終了すると、SSHペインに［このペインで再開］と［新しいペインで再開］が表示されます。利用者が選ぶまでsshcがセッションを自動的に再開することはありません。

再開時は、元の接続エイリアスから解決した同じSSH接続先で、そのエージェントの固定された再開コマンドを実行します。接続先の識別情報が変わった場合や候補が更新された場合は実行しません。ローカルシェルでは状態を表示できますが、sshcからのセッション再開には対応していません。

## Bridgeが扱う情報

BridgeがTerminalへ送るのは、次の情報だけです。

- エージェントの種類とライフサイクル状態
- ネイティブセッションID
- 作業ディレクトリ
- 公式フックから取得できる場合のモデルID
- 取得できた場合のセッション名

プロンプト、応答、ツールの入出力、認証情報、トランスクリプトのパスと本文は送りません。セッション名を探すため、CodexのセッションインデックスまたはClaude Code自身のトランスクリプトを末尾から最大1 MiBだけ読みます。該当するセッション名以外はTerminalへ含めません。

Bridgeはネットワーク通信や設定ファイルの書き換えを行わず、エージェントの許可判断も変えません。状態はエージェントを実行している端末デバイスへOSCとして書き込まれます。詳しい脅威モデルとデータの扱いは、Bridgeリポジトリの[Security policy](https://github.com/aida0710/sshc-agent-bridge/blob/main/SECURITY.md)を参照してください。

## 状態が表示されない場合

1. `sshc --version`がv0.19.0以降か確認します。
2. Bridgeを導入した後に、新しいエージェントセッションを開始します。
3. Codexでは`/hooks`を開き、フックが信頼済みか確認します。
4. Codex／Claude Codeでは、エージェントと同じ環境で`python3 --version`を実行できるか確認します。
5. SSH先やコンテナーで実行している場合は、その環境へBridgeを導入したか確認します。
6. Bridgeを最新版へ更新します。Claude Codeの端末解決はv0.1.3で修正されています。

フックが端末を見つけられない場合や未知のイベントを受け取った場合、Bridgeはエージェントの動作を止めずに終了します。そのため、エージェントが通常どおり動いていても状態だけが表示されないことがあります。

---
title: タイトルと通知
description: シェルやコーディングエージェントが送る標準のエスケープシーケンスで、ペイン名を更新し、入力待ちや完了を通知する。
---

# タイトルと通知

sshcのTerminalは、ターミナル内で動くプログラムが送る標準のエスケープシーケンスを解釈します。専用のプラグインやフックは必要ありません。macOSのTerminal.appやiTerm2でタブ名が変わる仕組みや、Claude Code、Codexが通知を出す仕組みを、そのままブラウザのsshcで使えます。

| 目的 | シーケンス | 送る側の例 |
|---|---|---|
| ペイン名を変える | OSC 0、OSC 1、OSC 2 | zsh／bashのプロンプト設定、vim、tmux、Claude Code、Codex |
| 通知を出す | OSC 9（iTerm2形式）、OSC 99（kitty形式）、OSC 777（rxvt／tmux形式） | Claude Code、Codex、`cmux notify`のような通知コマンド |

## ペイン名

プログラムが`ESC ] 0 ; 名前 BEL`のようなシーケンスでタイトルを設定すると、そのペインの見出し、セッション一覧、コマンドパレットの表示名が同じ名前になります。SSH先のシェルが送ったタイトルも、エンジンが受け取って同じように反映します。

- 利用者がペイン名を固定している間は、プログラムからのタイトルで上書きしません。［自動の名前に戻す］を選ぶと、再びプログラムのタイトルまたは接続エイリアスを表示します。
- プログラムがタイトルを空にすると、接続エイリアスまたはシェル名に戻ります。
- 再接続や再起動で新しいシェルが始まると、前のタイトルは消え、新しいシェルが送るタイトルを待ちます。
- 制御文字は取り除き、64文字までを表示します。

試すには、ペイン内で次を実行します。

```sh
printf '\e]0;hello\a'
```

## 通知

プログラムがOSC 9、OSC 99、OSC 777で通知を求めると、sshcはそのペインを未読として扱います。

- sshcのタブがバックグラウンドにあるか、別のペインを見ている間に届いた通知は、セッション一覧とワークスペースに未読の印を付けます。そのペインを表示すると消えます。
- タブがバックグラウンドにあるときは、［Settings］→［Notifications］で許可していればブラウザ通知を出し、選んだ通知音を鳴らします。ブラウザ通知の見出しはペイン名（SSHでは接続エイリアス付き）で、本文にはプログラムが送ったタイトルと本文を表示します。
- 表示中のペインに届いた通知は、未読にも通知音にもなりません。
- BEL（`\a`）だけの出力は通知として扱いません。補完の失敗などで頻繁に鳴るためです。

試すには、ペイン内で次のどれかを実行してから別のタブへ移ってください。

```sh
printf '\e]9;Task complete\a'
printf '\e]777;notify;Build finished;main.go compiled\a'
```

## Claude Codeで通知を受け取る

Claude Codeは、既定ではGhostty、Kitty、iTerm2を検出したときだけデスクトップ通知を送ります。sshcのTerminalはこの検出に含まれないため、`~/.claude/settings.json`で通知の送り方を明示します。`iterm2`はOSC 9、`kitty`はOSC 99を送り、どちらもsshcが受け取ります。

```json
{
  "preferredNotifChannel": "iterm2"
}
```

タスクの完了と権限確認の待ちが通知になります。Claude Codeがタスクの要約をタイトルに送る場合は、ペイン名にも反映されます。

## Codexで通知を受け取る

Codexの`~/.codex/config.toml`で、TUI通知を有効にし、送り方を`osc9`に固定します。`auto`のままだと、sshcをOSC 9対応と判定できずBELへ切り替わることがあります。

```toml
[tui]
notifications = true
notification_method = "osc9"
```

`notifications`には`["agent-turn-complete", "approval-requested"]`のように種類を並べて絞り込むこともできます。

## tmuxの中で使う

tmuxの中で動くプログラムからの通知やタイトルを外側のTerminalへ通すには、`~/.tmux.conf`に次を加えます。

```text
set -g allow-passthrough on
set -g set-titles on
```

## 反映されない場合

1. `sshc --version`がv0.35.0以降か確認します。
2. ペイン内で上の`printf`を実行し、ペイン名や未読の印が変わるか確認します。変わる場合、sshc側は動作しています。
3. Claude Code／Codexの設定を保存した後、新しいセッションを開始します。
4. SSH先でエージェントを動かしている場合、設定はその環境のホームディレクトリに置きます。
5. tmuxやscreenを挟んでいる場合は、パススルーの設定を確認します。

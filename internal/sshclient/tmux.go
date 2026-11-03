package sshclient

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// 接続先ごとの「tmuxで開く」。シェルの代わりに `tmux new -A -s <名前>` を起動するので、SSH が切れても
// プログラムはサーバーで動き続け、再接続や開き直しで同じ画面に戻れる。
//
// ssh_config の RemoteCommand には書かない。書くと `ssh host コマンド` や scp が断られ、tmux の無い
// サーバーでは接続がすぐ終わる。

// tmuxProbeLimit は、tmux を確かめるコマンド1本を待つ上限である。ログインシェルの設定を読むので
// 少し掛かることがあるが、セッションを開けるかどうかに関わらせない。
const tmuxProbeLimit = 10 * time.Second

// loginShellCommand は、コマンドをログインシェルの中で走らせる行である。ssh host コマンド は
// ログインシェルの設定を読まないので、Homebrew や ~/.local/bin の tmux が PATH に無い。line は
// シングルクォートを含まない。
func loginShellCommand(line string) string {
	return `exec "$SHELL" -lc '` + line + `'`
}

// openingInTmux は、接続先が tmux で開く設定なら、tmux を起動する RemoteCommand を載せた target を返す。
// 開けないときはそのことをターミナルに書き、シェルで開く。
func (s *Session) openingInTmux(ctx context.Context, client *ssh.Client, target Target) Target {
	name := target.TmuxSession
	if name == "" {
		return target
	}
	switch {
	case target.RemoteCommand != "":
		s.trace.announce("RemoteCommandがあるので、tmuxでは開きません。")
		return target
	case strings.EqualFold(target.RequestTTY, "no"):
		s.trace.announce("RequestTTY noなので、tmuxでは開きません。")
		return target
	}
	probe := tmuxProbe{ctx: ctx, client: client}
	if !probe.installed() {
		s.trace.announce("このサーバーにtmuxが見つからないので、通常のシェルで開きます。")
		return target
	}
	if probe.sessionRunning(name) {
		s.resumedTmux.Store(true)
		s.trace.announce("tmuxのセッション%sに戻ります。", name)
	} else {
		s.trace.announce("tmuxのセッション%sを新しく開きます。", name)
	}
	target.RemoteCommand = loginShellCommand("exec tmux new -A -s " + name)
	return target
}

// tmuxProbe は、シェルとは別のチャンネルでサーバーの tmux を確かめる。
type tmuxProbe struct {
	ctx    context.Context
	client *ssh.Client
}

func (p tmuxProbe) run(line string) (Output, error) {
	return runCommand(p.ctx, commandRun{
		client: p.client, command: Command{Line: loginShellCommand(line), Limit: tmuxProbeLimit},
		started: time.Now(), abandon: func() {},
	})
}

// installed は、tmux を起動できるかを返す。出力も見るのは、シェルの違いで終了コードが揃わないからである
// （Windows の cmd.exe は `exec` を知らず、ここで 1 を返す）。
func (p tmuxProbe) installed() bool {
	output, err := p.run("tmux -V")
	return err == nil && output.ExitCode == 0 && strings.Contains(string(output.Stdout), "tmux ")
}

// sessionRunning は、その名前の tmux のセッションが動いているかを返す。`=` は前方一致を止める。
func (p tmuxProbe) sessionRunning(name string) bool {
	output, err := p.run("tmux has-session -t =" + name)
	return err == nil && output.ExitCode == 0
}

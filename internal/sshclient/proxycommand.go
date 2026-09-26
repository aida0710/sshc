package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os/exec"

	"sshc/internal/commandconn"
)

// ProxyCommand は、ssh_config に記載された外部コマンドの標準入出力を SSH 接続に使う。
// 実行前にコマンドを利用者へ表示する。

// ErrProxyCommandThroughJump は、踏み台の向こうのホップが ProxyCommand を
// 持っている設定を断る。
//
// コマンドはローカルで実行されるため、踏み台から到達する後続ホップには適用できない。
var ErrProxyCommandThroughJump = errors.New(
	"a jump host reached through another connection cannot use ProxyCommand; the command would run on this machine")

// startProxyCommand は、その表記を起動し、その標準入出力を接続として返す。
//
// environment が nil なら、engine の環境をそのまま渡す。
func startProxyCommand(command string, environment []string, trace *tracer) (net.Conn, error) {
	name, arguments, err := interpreter(command)
	if err != nil {
		return nil, err
	}
	process := exec.Command(name, arguments...)
	process.Env = environment
	configureProxyCommandProcess(process, command)
	trace.say(Full, "ProxyCommandの実行シェル：%s", process.Path)
	if environment == nil {
		trace.say(Full, "ProxyCommandの環境：sshcの起動元から継承")
	}
	conn, err := commandconn.Start(process, command)
	if err != nil {
		return nil, fmt.Errorf("ProxyCommand did not start: %w", err)
	}
	trace.say(Full, "ProxyCommandを起動しました：PID %d", process.Process.Pid)
	return conn, nil
}

func describeProxyExit(trace *tracer, connection net.Conn) {
	command, ok := connection.(*commandconn.Conn)
	if !ok {
		return
	}
	select {
	case <-command.Exited():
		if err := command.ExitErr(); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				trace.say(Detailed, "ProxyCommandの終了：コード %d（%v）", exit.ExitCode(), err)
				return
			}
			trace.say(Detailed, "ProxyCommandの終了：%v", err)
			return
		}
		trace.say(Detailed, "ProxyCommandの終了：コード 0")
	default:
		trace.say(Detailed, "ProxyCommandの終了状態はまだ取得できていません。")
	}
}

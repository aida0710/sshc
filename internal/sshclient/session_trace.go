package sshclient

import (
	"errors"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

func describeSessionExit(trace *tracer, err error, elapsed time.Duration) {
	var exit *ssh.ExitError
	switch {
	case err == nil:
		trace.say(Detailed, "SSHセッションが終了しました：コード 0（実行時間 %s）。", connectionlog.Elapsed(elapsed))
	case errors.As(err, &exit):
		trace.say(Detailed, "SSHセッションが終了しました：コード %d、シグナル %q（実行時間 %s）。", exit.ExitStatus(), exit.Signal(), connectionlog.Elapsed(elapsed))
		if exit.Msg() != "" {
			trace.say(Full, "サーバーからの終了メッセージ：%s", terminal.DisplayText(exit.Msg(), maxBannerLineRunes))
		}
	default:
		trace.say(Detailed, "SSHセッションの終了状態を取得できませんでした（実行時間 %s）：%v", connectionlog.Elapsed(elapsed), err)
	}
}

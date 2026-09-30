package sshclient

import (
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

func describeSessionExit(trace *tracer, err error, elapsed time.Duration) {
	var exit *ssh.ExitError
	switch {
	case err == nil:
		trace.say(connectionlog.Detailed, "SSHセッションが終了しました：コード0（実行時間%s）。", connectionlog.Elapsed(elapsed))
	case errors.As(err, &exit):
		trace.say(connectionlog.Detailed, "SSHセッションが終了しました：コード%d、シグナル%q（実行時間%s）。", exit.ExitStatus(), exit.Signal(), connectionlog.Elapsed(elapsed))
		if exit.Msg() != "" {
			trace.say(connectionlog.Full, "サーバーからの終了メッセージ：%s", terminal.DisplayText(exit.Msg(), maxBannerLineRunes))
		}
	default:
		trace.say(connectionlog.Detailed, "SSHセッションの終了状態を取得できませんでした（実行時間%s）：%v", connectionlog.Elapsed(elapsed), err)
	}
}

// closingLog は、対話のセッションでシェルが終わるときの接続ログ（keepalive の上限、
// 終了状態）を溜める。
//
// 出力の終わりに書くと、終わったプログラム（tmux、vim）の代替画面に書かれ、engine が
// モードを戻したときに見えなくなる。溜めた行は ExitInfo.Notice の前に置き、engine は
// モードを戻したあとの通常の画面へ書く。keepalive の goroutine とシェルの終わりを待つ
// goroutine の両方が書くので、読み書きを mutex で守る。
type closingLog struct {
	mutex sync.Mutex
	lines strings.Builder
}

func (log *closingLog) Write(contents []byte) (int, error) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.lines.Write(contents)
}

// before は、溜めた行を notice の前に置いた文を返す。行が溜まっていれば、シェルの
// 出力の途中の行に続けないよう、改行してから書く。
func (log *closingLog) before(notice string) string {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	if log.lines.Len() == 0 {
		return notice
	}
	return "\r\n" + log.lines.String() + notice
}

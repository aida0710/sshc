package sshclient

import (
	"context"
	"io"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

// ConnectionObserver は、端末の無い接続（SFTP など）が接続するあいだに報告するものの
// 受け取り先である。Progress には各ホップの段階が、Log には接続ログの設定の深さで
// 書かれる行が届く。Log の行は、端末へ書くときと同じく CRLF で終わる。
type ConnectionObserver struct {
	Progress func(terminal.ConnectionProgress)
	Log      io.Writer
}

type connectionObserverKey struct{}

// WithConnectionObserver は、Dialer.Connect がその接続の経過を observer へ報告する ctx を返す。
// 接続を使い回して dial しない場合は、何も報告しない。
func WithConnectionObserver(ctx context.Context, observer ConnectionObserver) context.Context {
	return context.WithValue(ctx, connectionObserverKey{}, observer)
}

// observingTracer は、ctx に観測者があればその接続の tracer を返す。無ければ nil で、
// 何も書かない（これまでの端末の無い接続と同じ）。
func (d Dialer) observingTracer(ctx context.Context) *tracer {
	observer, ok := ctx.Value(connectionObserverKey{}).(ConnectionObserver)
	if !ok {
		return nil
	}
	return d.connectionTracer(observer.Log, observer.Progress)
}

// connectionTracer は、接続ログの設定の深さで writer へ書き、各ホップの段階を progress へ
// 伝える tracer を返す。設定は接続のたびに読む。
func (d Dialer) connectionTracer(writer io.Writer, progress func(terminal.ConnectionProgress)) *tracer {
	level := connectionlog.Notice
	if d.Verbosity != nil {
		level = d.Verbosity()
	}
	trace := newTracer(level, writer)
	trace.progress = progress
	trace.say(connectionlog.Full, "接続ログ：すべて（-vvv）")
	return trace
}

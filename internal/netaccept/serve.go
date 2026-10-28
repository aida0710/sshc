// Package netaccept は、待ち受けに届く接続を、待ち受けが閉じるまで受け付け続ける。
// VPN の中継と SSH のローカル転送が同じ受け付け方を使う。
package netaccept

import (
	"errors"
	"net"
	"time"
)

const (
	// firstRetryWait と retryWaitLimit は、受け付けが一時的に失敗したとき（ファイル記述子が
	// 尽きた、など）に待つ時間の最初と上限である。続けて失敗するたびに倍にする。
	firstRetryWait = 5 * time.Millisecond
	retryWaitLimit = time.Second
)

// Serve は、listener が閉じるまで接続を受け付け、それぞれを goroutine の handle に渡す。
//
// limit が 0 より大きければ、同時に処理する数をそこまでにし、超えた接続は goroutine を
// 作る前に閉じる。一時的な失敗で受け付けをやめると、待ち受けは開いたまま誰も受け取らない
// ので、少し待って受け付け直す。listener は net.Listen のもので、閉じたら net.ErrClosed を
// 返すこと（ほかの誤りは、閉じたとはみなさずに受け付け直す）。
func Serve(listener net.Listener, limit int, handle func(net.Conn)) {
	var active chan struct{}
	if limit > 0 {
		active = make(chan struct{}, limit)
	}
	wait := time.Duration(0)
	for {
		connection, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			wait = min(max(2*wait, firstRetryWait), retryWaitLimit)
			time.Sleep(wait)
			continue
		}
		wait = 0
		if active == nil {
			go handle(connection)
			continue
		}
		select {
		case active <- struct{}{}:
			go func() {
				defer func() { <-active }()
				handle(connection)
			}()
		default:
			_ = connection.Close()
		}
	}
}

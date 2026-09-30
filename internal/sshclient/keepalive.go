package sshclient

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
)

type keepAliveSettings struct {
	interval time.Duration
	count    int
	done     <-chan struct{}
	trace    *tracer
	// closingTrace は、上限に達して輸送を閉じるときの行の書き先である。対話の Session は
	// 出力ではなく ExitInfo.Notice へ回す（session_trace.go の closingLog）。nil なら trace へ書く。
	closingTrace *tracer
}

// unconfiguredReplyLimit は、keepalive を設定していない接続で、1 回の keepalive の
// 応答を待つ上限である。輸送が落ちていれば待たずに失敗するので、ここまで待つのは
// 応答の遅い相手だけである。OpenSSH の ServerAliveCountMax の既定（3 回）に
// 5 秒ずつを掛けた長さにする。
const unconfiguredReplyLimit = 15 * time.Second

// replyLimit は、輸送が生きているかを 1 回の keepalive で確かめるときに待つ上限で
// ある。keepalive を設定していれば、切断と判断するまでの時間（間隔×回数）と同じにする。
func (settings keepAliveSettings) replyLimit() time.Duration {
	if settings.interval <= 0 {
		return unconfiguredReplyLimit
	}
	return settings.interval * time.Duration(keepAliveCount(settings.count))
}

// keepAliveLoop closes the transport after the configured number of missed replies.
func keepAliveLoop(client *ssh.Client, settings keepAliveSettings) func() {
	if settings.interval <= 0 {
		return nil
	}
	count := keepAliveCount(settings.count)
	closing := settings.closingTrace
	if closing == nil {
		closing = settings.trace
	}
	return func() {
		ticker := time.NewTicker(settings.interval)
		defer ticker.Stop()
		missed := 0
		for {
			select {
			case <-settings.done:
				return
			case <-ticker.C:
			}
			started := settings.trace.now()
			settings.trace.say(connectionlog.Full, "keepaliveを送信します。")
			err := keepAliveReply(client, settings.interval, settings.done)
			if err == nil {
				settings.trace.say(connectionlog.Full, "keepaliveの応答を受信しました（%s）。", connectionlog.Elapsed(settings.trace.since(started)))
				missed = 0
				continue
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			missed++
			settings.trace.say(connectionlog.Detailed, "keepaliveの応答がありません（連続%d/%d回、%s）：%v", missed, count, connectionlog.Elapsed(settings.trace.since(started)), err)
			if missed >= count {
				closing.say(connectionlog.Brief, "keepaliveの連続失敗が上限に達したため、SSH接続を切断します。")
				_ = client.Close()
				return
			}
		}
	}
}

// OpenSSH's default when ServerAliveCountMax is unset.
const defaultKeepAliveCount = 3

func keepAliveCount(configured int) int {
	if configured <= 0 {
		return defaultKeepAliveCount
	}
	return configured
}

// A negative SSH reply still proves the peer is alive. Only transport errors or
// a missed deadline count as failure; a late reply must not block its goroutine.
func keepAliveReply(client *ssh.Client, limit time.Duration, done <-chan struct{}) error {
	answered := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		answered <- err
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case err := <-answered:
		return err
	case <-timer.C:
		return context.DeadlineExceeded
	case <-done:
		return context.Canceled
	}
}

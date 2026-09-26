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
}

// keepAliveLoop closes the transport after the configured number of missed replies.
func keepAliveLoop(client *ssh.Client, settings keepAliveSettings) func() {
	if settings.interval <= 0 {
		return nil
	}
	count := keepAliveCount(settings.count)
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
			settings.trace.say(Full, "keepaliveを送信します。")
			err := keepAliveReply(client, settings.interval, settings.done)
			if err == nil {
				settings.trace.say(Full, "keepaliveの応答を受信しました（%s）。", connectionlog.Elapsed(settings.trace.since(started)))
				missed = 0
				continue
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			missed++
			settings.trace.say(Detailed, "keepaliveの応答がありません（連続 %d/%d 回、%s）：%v", missed, count, connectionlog.Elapsed(settings.trace.since(started)), err)
			if missed >= count {
				settings.trace.say(Brief, "keepaliveの連続失敗が上限に達したため、SSH接続を切断します。")
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
func keepAliveReply(client *ssh.Client, interval time.Duration, done <-chan struct{}) error {
	answered := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		answered <- err
	}()
	timer := time.NewTimer(interval)
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

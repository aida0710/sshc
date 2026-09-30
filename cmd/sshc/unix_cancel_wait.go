//go:build unix

package main

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// unixCancelWake は ctx の取り消しを、poll で待てるパイプの読み取り側に変える。
// 補助 goroutine は 1 バイトを書くだけで、close はその終了を待ってからパイプを閉じる。
type unixCancelWake struct {
	read        *os.File
	write       *os.File
	stopWatcher chan struct{}
	watcherDone chan struct{}
}

func startUnixCancelWake(
	ctx context.Context, pipe func() (*os.File, *os.File, error),
) (*unixCancelWake, error) {
	read, write, err := pipe()
	if err != nil {
		return nil, err
	}
	wake := &unixCancelWake{
		read:        read,
		write:       write,
		stopWatcher: make(chan struct{}),
		watcherDone: make(chan struct{}),
	}
	go func() {
		defer close(wake.watcherDone)
		select {
		case <-ctx.Done():
			_, _ = write.Write([]byte{1})
		case <-wake.stopWatcher:
		}
	}()
	return wake, nil
}

func (wake *unixCancelWake) fd() int {
	return int(wake.read.Fd())
}

func (wake *unixCancelWake) close() {
	close(wake.stopWatcher)
	<-wake.watcherDone
	_ = wake.read.Close()
	_ = wake.write.Close()
}

// waitUnixReadable は inputFD が読めるようになるか、wakeFD が鳴る（ctx が取り消される）まで待つ。
// 端末を cooked のまま待てるので、Ctrl-C を SIGINT に変える端末でも読み取りの途中で抜けられる。
func waitUnixReadable(
	ctx context.Context, inputFD, wakeFD int, poll func([]unix.PollFd, int) (int, error),
) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready := []unix.PollFd{
			{Fd: int32(wakeFD), Events: unix.POLLIN},
			{Fd: int32(inputFD), Events: unix.POLLIN},
		}
		_, err := poll(ready, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if ready[0].Revents != 0 || ctx.Err() != nil {
			return context.Canceled
		}
		if ready[1].Revents&unix.POLLNVAL != 0 {
			return unix.EBADF
		}
		if ready[1].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0 {
			return nil
		}
	}
}

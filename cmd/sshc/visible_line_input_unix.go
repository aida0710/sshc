//go:build unix

package main

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
)

// unixVisibleInputWait は端末を cooked のまま、入力と取り消しのパイプを同じ poll で待つ。
type unixVisibleInputWait struct {
	ctx     context.Context
	inputFD int
	wake    *unixCancelWake
}

func startVisibleInputWait(ctx context.Context, input *os.File) (visibleInputWait, error) {
	wake, err := startUnixCancelWake(ctx, os.Pipe)
	if err != nil {
		return nil, err
	}
	return &unixVisibleInputWait{ctx: ctx, inputFD: int(input.Fd()), wake: wake}, nil
}

func (wait *unixVisibleInputWait) waitReadable() error {
	return waitUnixReadable(wait.ctx, wait.inputFD, wait.wake.fd(), unix.Poll)
}

// waitForLateCancel は待たない。Unix の cooked の端末では Ctrl-C は SIGINT になり、
// 読み取りを EOF で戻さないので、EOF は入力が本当に尽きたことを表す。
func (*unixVisibleInputWait) waitForLateCancel() {}

func (wait *unixVisibleInputWait) close() {
	wait.wake.close()
}

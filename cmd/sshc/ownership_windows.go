//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// windowsPeekInterval は、パイプの状態を見に行く間隔である。
//
// **PID を見に行くのではない。** 見ているのは所有権チャンネルそのものであり、
// これは Unix の poll に相当する手が Windows のパイプに無いことへの対処である。
const windowsPeekInterval = 100 * time.Millisecond

type windowsOwnership struct {
	handle windows.Handle
	cancel context.CancelFunc
	events chan error
	joined sync.WaitGroup
	closed sync.Once
}

func newOSOwnershipMonitor(reader io.Reader) (ownershipMonitor, error) {
	file, ok := reader.(*os.File)
	if !ok || file == nil {
		return nil, fmt.Errorf("%w: the ownership channel is not an operating-system channel", errOwnershipProtocol)
	}
	handle := windows.Handle(file.Fd())
	kind, err := windows.GetFileType(handle)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errOwnershipProtocol, err)
	}
	// コンソール、ディスク、キャラクタデバイスは寿命を伝えない。
	if kind != windows.FILE_TYPE_PIPE {
		return nil, fmt.Errorf("%w: the ownership channel is not a pipe", errOwnershipProtocol)
	}
	return &windowsOwnership{handle: handle}, nil
}

func (o *windowsOwnership) Start(ctx context.Context) (<-chan error, error) {
	if cause := o.inspect(); cause != nil {
		return nil, cause
	}
	watchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	o.cancel = cancel
	o.events = make(chan error, 1)
	o.joined.Add(1)
	go func() {
		defer o.joined.Done()
		o.events <- o.watch(watchCtx)
	}()
	go func() {
		<-ctx.Done()
		o.stopWatcher()
	}()
	return o.events, nil
}

// inspect は、いまパイプがどうなっているかを一度だけ見る。
//
// 読める中身があることは常に規約違反である。これは寿命だけを運ぶチャンネルで
// あって、命令の通り道ではない。
func (o *windowsOwnership) inspect() error {
	var available uint32
	err := windows.PeekNamedPipe(o.handle, nil, 0, nil, &available, nil)
	switch {
	case errors.Is(err, windows.ERROR_BROKEN_PIPE), errors.Is(err, windows.ERROR_NO_DATA):
		return errOwnershipEnded
	case err != nil:
		return fmt.Errorf("%w: %v", errOwnershipRead, err)
	case available > 0:
		return errOwnershipProtocol
	}
	return nil
}

func (o *windowsOwnership) watch(ctx context.Context) error {
	ticker := time.NewTicker(windowsPeekInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if cause := o.inspect(); cause != nil {
				return cause
			}
		}
	}
}

func (o *windowsOwnership) stopWatcher() {
	o.closed.Do(func() {
		if o.cancel != nil {
			o.cancel()
		}
	})
}

func (o *windowsOwnership) Stop() error {
	if o.events == nil {
		return nil
	}
	o.stopWatcher()
	o.joined.Wait()
	select {
	case cause := <-o.events:
		if errors.Is(cause, errOwnershipRead) {
			return cause
		}
		return nil
	default:
		return nil
	}
}

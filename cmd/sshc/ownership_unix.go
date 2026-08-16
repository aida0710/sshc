//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// unixOwnership は、FIFO または接続済みの AF_UNIX ストリームソケットを見張る。
//
// 設計の言う「pipe」は、抽象的な一方向の寿命チャンネルである。Electron が
// `stdio: "pipe"` で起こした子の fd 0 は、macOS では実測すると FIFO ではなく
// **接続済みの無名 AF_UNIX SOCK_STREAM** である。FIFO だけを受け付ける実装は、
// デスクトップの起動をそのまま失敗させる。
type unixOwnership struct {
	descriptor int
	socket     bool

	// cancelRead と cancelWrite は、監視を止めるための自前の合図である。
	// **stdin は閉じない。** 閉じれば、呼び出し側の持ち物を勝手に壊すことになる。
	cancelRead  int
	cancelWrite int

	events chan error
	closed sync.Once
	joined sync.WaitGroup
}

func newOSOwnershipMonitor(reader io.Reader) (ownershipMonitor, error) {
	file, ok := reader.(*os.File)
	if !ok || file == nil {
		return nil, fmt.Errorf("%w: the ownership channel is not an operating-system channel", errOwnershipProtocol)
	}
	descriptor := int(file.Fd())
	var status unix.Stat_t
	if err := unix.Fstat(descriptor, &status); err != nil {
		return nil, fmt.Errorf("%w: %v", errOwnershipProtocol, err)
	}
	switch status.Mode & unix.S_IFMT {
	case unix.S_IFIFO:
		return &unixOwnership{descriptor: descriptor}, nil
	case unix.S_IFSOCK:
		if err := verifyUnixStreamPeer(descriptor); err != nil {
			return nil, err
		}
		return &unixOwnership{descriptor: descriptor, socket: true}, nil
	}
	// 端末、通常ファイル、ディレクトリ、デバイス。どれも寿命を伝えない。
	return nil, fmt.Errorf("%w: the ownership channel is not a pipe or a Unix stream socket", errOwnershipProtocol)
}

// verifyUnixStreamPeer は、接続済みの AF_UNIX ストリームだけを通す。
//
// 無名の相手は正常である——Electron の起こす対には名前が無い。拒むのは、
// 繋がっていないソケットと、SOCK_STREAM であっても AF_INET/AF_INET6 のものである。
func verifyUnixStreamPeer(descriptor int) error {
	kind, err := unix.GetsockoptInt(descriptor, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil {
		return fmt.Errorf("%w: %v", errOwnershipProtocol, err)
	}
	if kind != unix.SOCK_STREAM {
		return fmt.Errorf("%w: the ownership socket is not a stream", errOwnershipProtocol)
	}
	peer, err := unix.Getpeername(descriptor)
	if err != nil {
		return fmt.Errorf("%w: the ownership socket is not connected", errOwnershipProtocol)
	}
	if _, ok := peer.(*unix.SockaddrUnix); !ok {
		return fmt.Errorf("%w: the ownership socket is not a Unix peer", errOwnershipProtocol)
	}
	return nil
}

func (o *unixOwnership) Start(ctx context.Context) (<-chan error, error) {
	// **開始前の状態を同期的に確かめてから返る。** ここで見たものが「開始前
	// EOF」であり、それはロックを取る前でなければならない。この検査のあとに
	// 相手が閉じたものは実行中の所有権喪失であり、エンジンが一瞬ロックを取って
	// すぐ手放すことはありうる。チェックとロックを原子的にする手は無い。
	if cause := o.inspect(); cause != nil {
		return nil, cause
	}

	read, write, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errOwnershipRead, err)
	}
	o.cancelRead, o.cancelWrite = int(read.Fd()), int(write.Fd())
	// ファイルそのものは監視の寿命に縛る。fd だけを poll に渡す。
	o.events = make(chan error, 1)
	o.joined.Add(1)
	go func() {
		defer o.joined.Done()
		defer read.Close()
		defer write.Close()
		o.events <- o.watch()
	}()
	go func() {
		<-ctx.Done()
		o.stopWatcher()
	}()
	return o.events, nil
}

// inspect は、いまチャンネルがどうなっているかを一度だけ見る。
func (o *unixOwnership) inspect() error {
	fds := []unix.PollFd{{Fd: int32(o.descriptor), Events: unix.POLLIN}}
	if _, err := poll(fds, 0); err != nil {
		return fmt.Errorf("%w: %v", errOwnershipRead, err)
	}
	return o.classify(fds[0].Revents)
}

// classify は、poll の答えを所有権の言葉に直す。
//
// 中身があることは常に規約違反である。これは寿命だけを運ぶチャンネルであって、
// 命令の通り道ではない。
func (o *unixOwnership) classify(revents int16) error {
	switch {
	case revents&unix.POLLNVAL != 0:
		return fmt.Errorf("%w: the ownership channel is not open", errOwnershipRead)
	case revents&unix.POLLERR != 0:
		return fmt.Errorf("%w: the ownership channel reported an error", errOwnershipRead)
	case revents&unix.POLLIN != 0:
		if !o.socket {
			// FIFO は、書き手が閉じれば POLLHUP を返す。POLLIN は中身である。
			return errOwnershipProtocol
		}
		// ソケットは、相手が閉じても中身が来ても POLLIN を返す。**消費せずに**
		// 覗いて分ける。
		buffer := make([]byte, 1)
		read, _, err := unix.Recvfrom(o.descriptor, buffer, unix.MSG_PEEK|unix.MSG_DONTWAIT)
		switch {
		case err != nil && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK):
			return fmt.Errorf("%w: %v", errOwnershipRead, err)
		case read > 0:
			return errOwnershipProtocol
		case read == 0 && err == nil:
			return errOwnershipEnded
		}
		if revents&unix.POLLHUP != 0 {
			return errOwnershipEnded
		}
		return nil
	case revents&unix.POLLHUP != 0:
		return errOwnershipEnded
	}
	// 生きていて、空である。これが正常な状態である。
	return nil
}

func (o *unixOwnership) watch() error {
	for {
		fds := []unix.PollFd{
			{Fd: int32(o.descriptor), Events: unix.POLLIN},
			{Fd: int32(o.cancelRead), Events: unix.POLLIN},
		}
		ready, err := poll(fds, -1)
		if err != nil {
			return fmt.Errorf("%w: %v", errOwnershipRead, err)
		}
		if ready == 0 {
			continue
		}
		if fds[1].Revents != 0 {
			return nil
		}
		if fds[0].Revents == 0 {
			continue
		}
		if cause := o.classify(fds[0].Revents); cause != nil {
			return cause
		}
	}
}

// poll は、シグナルで中断された待機を、期限を延ばさずにやり直す。
func poll(fds []unix.PollFd, timeout int) (int, error) {
	for {
		ready, err := unix.Poll(fds, timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return ready, err
	}
}

func (o *unixOwnership) stopWatcher() {
	o.closed.Do(func() {
		// 1 バイトで監視を起こす。閉じるのはこちらの持ち物だけである。
		_, _ = unix.Write(o.cancelWrite, []byte{0})
	})
}

func (o *unixOwnership) Stop() error {
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

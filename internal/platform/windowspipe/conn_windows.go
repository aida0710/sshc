//go:build windows

// Package windowspipe は、Windows の named pipe を net.Conn として通信する。
//
// これは agent のためにある。Unix のこのアプリケーションが SSH_AUTH_SOCK
// の unix ソケットへ繋ぐところで、Windows の OpenSSH は固定の named pipe を
// 待っている。プロトコルは同じなので、必要なのは運ぶ管だけである。
//
// x/sys だけで書く。named pipe のために依存をひとつ増やすと、鍵とパス
// フレーズが通る経路に、このプロジェクトが読んでいないコードが入る。
package windowspipe

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AgentPipe は、Windows の OpenSSH エージェントが待っている場所である。
//
// 環境変数で差し替えない。Unix の SSH_AUTH_SOCK は OpenSSH 自身の約束
// だが、Windows にその約束は無い。読める変数をひとつでも見れば、それは
// 「鍵とパスフレーズを任意のパイプへ渡す方法」になる。名前はひとつだけである。
const AgentPipe = `\\.\pipe\openssh-ssh-agent`

// pipeBusyRetry は named pipe の全インスタンスが使用中の場合の再試行間隔である。
//
// OpenSSH agent は接続ごとに pipe instance を再作成するため、使用中の場合は
// ERROR_PIPE_BUSY が返ることがある。そこで諦めると、二本目の接続がたまたま
// 失敗する。
const pipeBusyRetry = 100 * time.Millisecond

// waitNamedPipe は kernel32 が持つ。x/sys が包んでいないので自分で引く。
var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	waitNamedPipe = kernel32.NewProc("WaitNamedPipeW")
)

// DialContext は、named pipe をひとつ開く。
//
// 開くまでの待ちも ctx に従う。パイプが埋まっているときの再試行で、
// キャンセルされた呼び出し元を待たせない。
func DialContext(ctx context.Context, path string) (net.Conn, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: "pipe", Err: err}
	}
	for {
		handle, err := windows.CreateFile(
			name,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil,
			windows.OPEN_EXISTING,
			// overlapped でなければ、待っている読み書きを解除する手段が無い。
			// 締切もキャンセルも、そこから作られる。
			windows.FILE_FLAG_OVERLAPPED,
			0,
		)
		if err == nil {
			return newConn(handle, path)
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, &net.OpError{Op: "dial", Net: "pipe", Err: err}
		}
		if err := waitForPipe(ctx, name); err != nil {
			return nil, &net.OpError{Op: "dial", Net: "pipe", Err: err}
		}
	}
}

// waitForPipe は pipe instance が利用可能になるまで待機する。
func waitForPipe(ctx context.Context, name *uint16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wait := pipeBusyRetry
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
	}
	if wait <= 0 {
		return context.DeadlineExceeded
	}
	// 戻り値が 0 なら待ちきれなかっただけである。次の CreateFile が本当の
	// 理由を返すので、ここでは区別せずに戻る。
	_, _, _ = waitNamedPipe.Call(uintptr(unsafe.Pointer(name)), uintptr(wait/time.Millisecond))
	return ctx.Err()
}

// conn は、開いた named pipe ひとつである。
//
// 読みと書きは別の event を持つ。ひとつを共有すると、同時に走った読みと
// 書きが互いの完了を待ってしまう。net.Conn は両方を同時に呼ばれてよい。
type conn struct {
	address pipeAddress

	closeOnce sync.Once
	closed    chan struct{}

	handle windows.Handle

	read  operation
	write operation
}

// operation は、片方向の重なり合う入出力ひとつぶんの状態である。
//
// overlapped も buffer も、この構造体が持つ。呼び出し元の slice をその
// まま渡すと、kernel が書き込んでいる最中に Go のスタックが動きうる。ヒープ
// 上のこの領域は動かないので、そこを経由して写す。
type operation struct {
	mutex      sync.Mutex
	event      windows.Handle
	overlapped windows.Overlapped
	buffer     [maxPipeMessage]byte

	deadlineMutex sync.Mutex
	deadline      time.Time
}

// maxPipeMessage は、一度に運ぶ最大の長さである。agent のメッセージはこれより
// ずっと小さい。鍵ひとつぶんの blob が通れば足りる。
const maxPipeMessage = 32 << 10

func newConn(handle windows.Handle, path string) (net.Conn, error) {
	pipe := &conn{
		address: pipeAddress(path),
		closed:  make(chan struct{}),
		handle:  handle,
	}
	for _, operation := range []*operation{&pipe.read, &pipe.write} {
		// manual reset の event を使う。auto reset だと、待たずに完了した
		// 入出力の合図を取りこぼす。
		event, err := windows.CreateEvent(nil, 1, 0, nil)
		if err != nil {
			_ = pipe.Close()
			return nil, &net.OpError{Op: "dial", Net: "pipe", Err: err}
		}
		operation.event = event
		operation.overlapped.HEvent = event
	}
	return pipe, nil
}

func (pipe *conn) Read(buffer []byte) (int, error) {
	read, err := pipe.perform(&pipe.read, "read", buffer, false)
	if err == nil && read == 0 && len(buffer) != 0 {
		return 0, io.EOF
	}
	return read, err
}

// Write は、渡されたぶんを書き切る。
//
// 一度の重なり合う書き込みには上限がある。io.Writer は、書けた長さが
// 短いなら error を返せと言う。暗黙に短く返すと、agent のメッセージが途中で
// 切れたまま「成功」として扱われる。上限より長いものは分けて運ぶ。
func (pipe *conn) Write(buffer []byte) (int, error) {
	written := 0
	for written < len(buffer) {
		count, err := pipe.perform(&pipe.write, "write", buffer[written:], true)
		written += count
		if err != nil {
			return written, err
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

// perform は、重なり合う入出力ひとつを最後まで面倒を見る。
//
// 締切で解けたときは CancelIoEx を投げ、そのうえで結果を回収する。
// 回収しないまま戻ると、kernel はまだこの構造体へ書き込みうる。
func (pipe *conn) perform(operation *operation, verb string, buffer []byte, writing bool) (int, error) {
	select {
	case <-pipe.closed:
		return 0, pipe.opError(verb, net.ErrClosed)
	default:
	}

	operation.mutex.Lock()
	defer operation.mutex.Unlock()

	length := len(buffer)
	if length > len(operation.buffer) {
		length = len(operation.buffer)
	}
	if writing {
		copy(operation.buffer[:length], buffer[:length])
	}

	if err := windows.ResetEvent(operation.event); err != nil {
		return 0, pipe.opError(verb, err)
	}
	var done uint32
	var err error
	if writing {
		err = windows.WriteFile(pipe.handle, operation.buffer[:length], &done, &operation.overlapped)
	} else {
		err = windows.ReadFile(pipe.handle, operation.buffer[:length], &done, &operation.overlapped)
	}
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) {
			return 0, io.EOF
		}
		return 0, pipe.opError(verb, err)
	}
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		done, err = pipe.awaitCompletion(operation, verb)
		if err != nil {
			return int(done), err
		}
	}
	if !writing {
		copy(buffer[:done], operation.buffer[:done])
	}
	return int(done), nil
}

// awaitCompletion は、入出力が終わるか、締切か閉鎖が来るまで待つ。
func (pipe *conn) awaitCompletion(operation *operation, verb string) (uint32, error) {
	timeout := operation.remaining()
	if timeout == 0 {
		return pipe.cancel(operation, verb, os.ErrDeadlineExceeded)
	}

	waited, err := windows.WaitForSingleObject(operation.event, timeout)
	switch {
	case err != nil:
		return pipe.cancel(operation, verb, err)
	case waited == uint32(windows.WAIT_TIMEOUT):
		return pipe.cancel(operation, verb, os.ErrDeadlineExceeded)
	}

	var done uint32
	if err := windows.GetOverlappedResult(pipe.handle, &operation.overlapped, &done, false); err != nil {
		if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) {
			return done, io.EOF
		}
		if errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
			return done, pipe.abortReason(verb)
		}
		return done, pipe.opError(verb, err)
	}
	return done, nil
}

// cancel は、待つのをやめた入出力を kernel から取り下げ、回収する。
func (pipe *conn) cancel(operation *operation, verb string, reason error) (uint32, error) {
	_ = windows.CancelIoEx(pipe.handle, &operation.overlapped)
	var done uint32
	// wait=true で回収する。取り下げたことと、kernel が手を引いたことは
	// 別である。回収せずに戻れば、この構造体はまだ書き換えられうる。
	_ = windows.GetOverlappedResult(pipe.handle, &operation.overlapped, &done, true)
	return done, pipe.opError(verb, reason)
}

// abortReason は、取り下げられた入出力に理由を与える。閉じたのか、締切なのか。
func (pipe *conn) abortReason(verb string) error {
	select {
	case <-pipe.closed:
		return pipe.opError(verb, net.ErrClosed)
	default:
		return pipe.opError(verb, os.ErrDeadlineExceeded)
	}
}

func (pipe *conn) opError(verb string, err error) error {
	return &net.OpError{Op: verb, Net: "pipe", Addr: pipe.address, Err: err}
}

// Close は、待っている入出力ごと閉じる。
//
// CancelIoEx を先に投げる。閉じるだけでは、この handle で待っている読みは
// 解けない。解けないまま CloseHandle すると、kernel が触っている最中の
// handle を無効にすることになる。
func (pipe *conn) Close() error {
	var err error
	pipe.closeOnce.Do(func() {
		close(pipe.closed)
		if pipe.handle != 0 {
			_ = windows.CancelIoEx(pipe.handle, nil)
			err = windows.CloseHandle(pipe.handle)
			pipe.handle = 0
		}
		for _, operation := range []*operation{&pipe.read, &pipe.write} {
			if operation.event != 0 {
				_ = windows.CloseHandle(operation.event)
				operation.event = 0
			}
		}
	})
	return err
}

func (pipe *conn) LocalAddr() net.Addr  { return pipe.address }
func (pipe *conn) RemoteAddr() net.Addr { return pipe.address }

func (pipe *conn) SetDeadline(at time.Time) error {
	pipe.read.setDeadline(at)
	pipe.write.setDeadline(at)
	return nil
}

func (pipe *conn) SetReadDeadline(at time.Time) error {
	pipe.read.setDeadline(at)
	return nil
}

func (pipe *conn) SetWriteDeadline(at time.Time) error {
	pipe.write.setDeadline(at)
	return nil
}

func (operation *operation) setDeadline(at time.Time) {
	operation.deadlineMutex.Lock()
	defer operation.deadlineMutex.Unlock()
	operation.deadline = at
}

// remaining は、待ってよい残りをミリ秒で返す。締切が無ければ無限を返す。
// 0 は「もう過ぎている」であり、待たずに取り下げる合図である。
func (operation *operation) remaining() uint32 {
	operation.deadlineMutex.Lock()
	deadline := operation.deadline
	operation.deadlineMutex.Unlock()
	if deadline.IsZero() {
		return windows.INFINITE
	}
	left := time.Until(deadline)
	if left <= 0 {
		return 0
	}
	milliseconds := left.Milliseconds()
	if milliseconds >= int64(windows.INFINITE) {
		return windows.INFINITE - 1
	}
	// 1ms 未満の残りを 0 に丸めると「もう過ぎている」と読まれる。
	if milliseconds == 0 {
		return 1
	}
	return uint32(milliseconds)
}

// pipeAddress は、この接続の宛先である。資格情報を持たない。
// 名前は固定であり、そこに秘密は現れない。
type pipeAddress string

func (address pipeAddress) Network() string { return "pipe" }
func (address pipeAddress) String() string  { return string(address) }

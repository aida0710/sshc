//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type unixPasswordOperations struct {
	makeRaw func(int) (*term.State, error)
	restore func(int, *term.State) error
	pipe    func() (*os.File, *os.File, error)
	poll    func([]unix.PollFd, int) (int, error)
	read    func(int, []byte) (int, error)
}

func systemUnixPasswordOperations() unixPasswordOperations {
	return unixPasswordOperations{
		makeRaw: term.MakeRaw,
		restore: term.Restore,
		pipe:    os.Pipe,
		poll:    unix.Poll,
		read:    unix.Read,
	}
}

func (systemPasswordTerminal) ReadPassword(
	ctx context.Context, input *os.File, prompt func() error,
) ([]byte, error) {
	return readUnixPasswordWithFeedback(ctx, input, systemUnixPasswordOperations(), prompt, nil)
}

func (systemPasswordTerminal) ReadPasswordMasked(
	ctx context.Context, input *os.File, prompt func() error, feedback func(int) error,
) ([]byte, error) {
	return readUnixPasswordWithFeedback(ctx, input, systemUnixPasswordOperations(), prompt, feedback)
}

// readUnixPassword は端末とキャンセルパイプを同じ poll で待つ。補助 goroutine は
// 1 バイトを書くだけで、保存済み端末モードを復元する前に終了を待つ。呼び出し側の
// stdin を所有せず、閉じることもない。
func readUnixPassword(
	ctx context.Context, input *os.File, operations unixPasswordOperations,
) (password []byte, resultErr error) {
	return readUnixPasswordWithFeedback(ctx, input, operations, nil, nil)
}

func readUnixPasswordWithPrompt(
	ctx context.Context, input *os.File, operations unixPasswordOperations, prompt func() error,
) (password []byte, resultErr error) {
	return readUnixPasswordWithFeedback(ctx, input, operations, prompt, nil)
}

func readUnixPasswordWithFeedback(
	ctx context.Context,
	input *os.File,
	operations unixPasswordOperations,
	prompt func() error,
	feedback func(int) error,
) (password []byte, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd := int(input.Fd())
	saved, err := operations.makeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := operations.restore(fd, saved); err != nil {
			zeroBytes(password)
			password = nil
			restoreErr := fmt.Errorf("restore password terminal mode: %w", err)
			if resultErr != nil {
				resultErr = errors.Join(resultErr, restoreErr)
			} else {
				resultErr = restoreErr
			}
		}
	}()

	wakeRead, wakeWrite, err := operations.pipe()
	if err != nil {
		return nil, err
	}
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_, _ = wakeWrite.Write([]byte{1})
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		<-watcherDone
		_ = wakeRead.Close()
		_ = wakeWrite.Close()
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prompt != nil {
		if err := prompt(); err != nil {
			return nil, err
		}
	}
	password, err = readUnixPasswordBytesWithFeedback(ctx, fd, int(wakeRead.Fd()), operations, feedback)
	if err != nil {
		zeroBytes(password)
		return nil, err
	}
	return password, nil
}

func readUnixPasswordBytes(
	ctx context.Context, terminalFD, wakeFD int, operations unixPasswordOperations,
) ([]byte, error) {
	return readUnixPasswordBytesWithFeedback(ctx, terminalFD, wakeFD, operations, nil)
}

func readUnixPasswordBytesWithFeedback(
	ctx context.Context,
	terminalFD, wakeFD int,
	operations unixPasswordOperations,
	feedback func(int) error,
) ([]byte, error) {
	// 固定容量のバッファを使い、append によるスライス拡張で古いヒープ領域へ
	// パスワードのコピーを残さないようにする。
	password := make([]byte, 0, maxVaultPasswordBytes)
	reportedRunes := 0
	for {
		if err := ctx.Err(); err != nil {
			zeroBytes(password)
			return nil, err
		}
		ready := []unix.PollFd{
			{Fd: int32(wakeFD), Events: unix.POLLIN},
			{Fd: int32(terminalFD), Events: unix.POLLIN},
		}
		_, err := operations.poll(ready, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			zeroBytes(password)
			return nil, err
		}
		if ready[0].Revents != 0 || ctx.Err() != nil {
			zeroBytes(password)
			return nil, context.Canceled
		}
		if ready[1].Revents&unix.POLLNVAL != 0 {
			zeroBytes(password)
			return nil, unix.EBADF
		}
		if ready[1].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
			continue
		}
		var input [1]byte
		count, readErr := operations.read(terminalFD, input[:])
		if count > 0 {
			finished, editErr := consumeUnixPasswordByte(&password, input[0])
			if editErr != nil {
				zeroBytes(password)
				return nil, editErr
			}
			if feedback != nil && utf8.Valid(password) {
				after := utf8.RuneCount(password)
				if after != reportedRunes {
					if feedbackErr := feedback(after); feedbackErr != nil {
						zeroBytes(password)
						return nil, feedbackErr
					}
					reportedRunes = after
				}
			}
			if finished {
				if err := ctx.Err(); err != nil {
					zeroBytes(password)
					return nil, err
				}
				if !utf8.Valid(password) {
					zeroBytes(password)
					return nil, errInvalidPasswordText
				}
				return password, nil
			}
		}
		if readErr != nil {
			zeroBytes(password)
			return nil, readErr
		}
		if count == 0 {
			zeroBytes(password)
			return nil, io.EOF
		}
	}
}

func consumeUnixPasswordByte(password *[]byte, value byte) (bool, error) {
	switch value {
	case '\n', '\r':
		return true, nil
	case 0x03:
		return false, context.Canceled
	case 0x04:
		if len(*password) == 0 {
			return false, io.EOF
		}
		return true, nil
	case '\b', 0x7f:
		*password = eraseLastPasswordRune(*password)
		return false, nil
	case 0x15:
		zeroBytes(*password)
		*password = (*password)[:0]
		return false, nil
	default:
		if value < 0x20 {
			return false, nil
		}
		if len(*password) >= maxVaultPasswordBytes {
			return false, errVaultPasswordTooLong
		}
		*password = append(*password, value)
		return false, nil
	}
}

func eraseLastPasswordRune(password []byte) []byte {
	if len(password) == 0 {
		return password
	}
	start := len(password) - 1
	for start > 0 && !utf8.RuneStart(password[start]) {
		start--
	}
	if !utf8.Valid(password[start:]) {
		start = len(password) - 1
	}
	zeroBytes(password[start:])
	return password[:start]
}

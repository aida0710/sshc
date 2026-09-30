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

// readUnixPasswordWithFeedback は、エコーを止めたターミナルからパスワードを 1 行読む。
//
// ターミナルとキャンセルパイプを同じ poll で待つ（waitUnixReadable）ので、ctx が
// 止まれば入力の途中でも戻る。ターミナルのモードは defer で必ず戻す。呼び出し側の
// stdin を所有せず、閉じることもない。prompt はエコーを止めた後、読む前に呼ぶ。
// feedback は、入力した文字数が変わるたびにその数を受け取る。どちらも nil でよい。
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

	wake, err := startUnixCancelWake(ctx, operations.pipe)
	if err != nil {
		return nil, err
	}
	defer wake.close()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prompt != nil {
		if err := prompt(); err != nil {
			return nil, err
		}
	}
	password, err = readUnixPasswordBytesWithFeedback(ctx, fd, wake.fd(), operations, feedback)
	if err != nil {
		zeroBytes(password)
		return nil, err
	}
	return password, nil
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
		if err := waitUnixReadable(ctx, terminalFD, wakeFD, operations.poll); err != nil {
			zeroBytes(password)
			return nil, err
		}
		var input [1]byte
		count, readErr := operations.read(terminalFD, input[:])
		if count > 0 {
			finished, editErr := consumeUnixPasswordByte(&password, input[0])
			if editErr != nil {
				zeroBytes(password)
				return nil, editErr
			}
			if feedbackErr := reportPasswordRunes(password, &reportedRunes, feedback); feedbackErr != nil {
				zeroBytes(password)
				return nil, feedbackErr
			}
			if finished {
				return finishUnixPassword(ctx, password)
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

// reportPasswordRunes は、入力が正しい UTF-8 で、文字数が前に知らせた数から変わった
// ときだけ、feedback に文字数を知らせる。
func reportPasswordRunes(password []byte, reported *int, feedback func(int) error) error {
	if feedback == nil || !utf8.Valid(password) {
		return nil
	}
	count := utf8.RuneCount(password)
	if count == *reported {
		return nil
	}
	if err := feedback(count); err != nil {
		return err
	}
	*reported = count
	return nil
}

// finishUnixPassword は、確定した入力を返す。取り消されていたか、正しい UTF-8 で
// なければ、入力を消してから断る。
func finishUnixPassword(ctx context.Context, password []byte) ([]byte, error) {
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

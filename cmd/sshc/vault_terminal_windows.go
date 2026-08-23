//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsKeyInput struct {
	keyDown         bool
	repeat          uint16
	virtualKey      uint16
	unicodeChar     uint16
	controlKeyState uint32
}

type windowsPasswordOperations struct {
	getMode     func(windows.Handle) (uint32, error)
	setMode     func(windows.Handle, uint32) error
	createEvent func() (windows.Handle, error)
	setEvent    func(windows.Handle) error
	closeHandle func(windows.Handle) error
	wait        func([]windows.Handle) (uint32, error)
	readInput   func(windows.Handle) (windowsKeyInput, error)
}

func systemWindowsPasswordOperations() windowsPasswordOperations {
	return windowsPasswordOperations{
		getMode: func(handle windows.Handle) (uint32, error) {
			var mode uint32
			err := windows.GetConsoleMode(handle, &mode)
			return mode, err
		},
		setMode: windows.SetConsoleMode,
		createEvent: func() (windows.Handle, error) {
			return windows.CreateEvent(nil, 1, 0, nil)
		},
		setEvent:    windows.SetEvent,
		closeHandle: windows.CloseHandle,
		wait: func(handles []windows.Handle) (uint32, error) {
			return windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		},
		readInput: readWindowsConsoleInput,
	}
}

func (systemPasswordTerminal) ReadPassword(ctx context.Context, input *os.File) ([]byte, error) {
	return readWindowsPassword(ctx, windows.Handle(input.Fd()), systemWindowsPasswordOperations())
}

// readWindowsPassword はコンソール入力とキャンセルイベントを同時に待つ。
// 補助 goroutine は専用イベントを通知するだけで、イベントを閉じて保存済みの
// コンソールモードを復元する前に終了を待つ。
func readWindowsPassword(
	ctx context.Context, input windows.Handle, operations windowsPasswordOperations,
) (password []byte, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	saved, err := operations.getMode(input)
	if err != nil {
		return nil, err
	}
	raw := saved &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)
	if err := operations.setMode(input, raw); err != nil {
		return nil, err
	}
	defer func() {
		if err := operations.setMode(input, saved); err != nil {
			zeroBytes(password)
			password = nil
			restoreErr := fmt.Errorf("restore password console mode: %w", err)
			if resultErr != nil {
				resultErr = errors.Join(resultErr, restoreErr)
			} else {
				resultErr = restoreErr
			}
		}
	}()

	cancelEvent, err := operations.createEvent()
	if err != nil {
		return nil, err
	}
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = operations.setEvent(cancelEvent)
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		<-watcherDone
		_ = operations.closeHandle(cancelEvent)
	}()

	units := make([]uint16, 0, maxVaultPasswordBytes)
	defer zeroUint16s(units)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ready, err := operations.wait([]windows.Handle{cancelEvent, input})
		if err != nil {
			return nil, err
		}
		if ready == windows.WAIT_OBJECT_0 || ctx.Err() != nil {
			return nil, context.Canceled
		}
		if ready != windows.WAIT_OBJECT_0+1 {
			return nil, fmt.Errorf("wait for password console input: unexpected result %#x", ready)
		}
		key, err := operations.readInput(input)
		if err != nil {
			return nil, err
		}
		finished, err := consumeWindowsPasswordKey(&units, key)
		if err != nil {
			return nil, err
		}
		if !finished {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		password, err = encodeWindowsPassword(units)
		if err != nil {
			zeroBytes(password)
			return nil, err
		}
		return password, nil
	}
}

func consumeWindowsPasswordKey(units *[]uint16, key windowsKeyInput) (bool, error) {
	if !key.keyDown {
		return false, nil
	}
	control := key.controlKeyState&(windows.LEFT_CTRL_PRESSED|windows.RIGHT_CTRL_PRESSED) != 0
	switch {
	case key.virtualKey == windows.VK_RETURN || key.unicodeChar == '\r' || key.unicodeChar == '\n':
		return true, nil
	case key.unicodeChar == 0x03 || (control && (key.virtualKey == 'C' || key.virtualKey == 'c')):
		return false, context.Canceled
	case key.unicodeChar == 0x04 || (control && (key.virtualKey == 'D' || key.virtualKey == 'd')):
		if len(*units) == 0 {
			return false, io.EOF
		}
		return true, nil
	case key.virtualKey == windows.VK_BACK || key.virtualKey == windows.VK_DELETE ||
		key.unicodeChar == '\b' || key.unicodeChar == 0x7f:
		for count := 0; count < windowsKeyRepeat(key); count++ {
			*units = eraseLastWindowsPasswordRune(*units)
		}
		return false, nil
	case key.unicodeChar == 0x15 || (control && (key.virtualKey == 'U' || key.virtualKey == 'u')):
		zeroUint16s(*units)
		*units = (*units)[:0]
		return false, nil
	case key.unicodeChar < 0x20:
		return false, nil
	}

	repeat := windowsKeyRepeat(key)
	if len(*units)+repeat > maxVaultPasswordBytes {
		return false, errVaultPasswordTooLong
	}
	for count := 0; count < repeat; count++ {
		*units = append(*units, key.unicodeChar)
	}
	return false, nil
}

func windowsKeyRepeat(key windowsKeyInput) int {
	if key.repeat == 0 {
		return 1
	}
	return int(key.repeat)
}

func eraseLastWindowsPasswordRune(units []uint16) []uint16 {
	if len(units) == 0 {
		return units
	}
	start := len(units) - 1
	if start > 0 && utf16.IsSurrogate(rune(units[start])) && units[start] >= 0xdc00 && units[start-1] >= 0xd800 && units[start-1] <= 0xdbff {
		start--
	}
	zeroUint16s(units[start:])
	return units[:start]
}

func encodeWindowsPassword(units []uint16) ([]byte, error) {
	password := make([]byte, 0, maxVaultPasswordBytes)
	for index := 0; index < len(units); index++ {
		value := rune(units[index])
		if value >= 0xd800 && value <= 0xdbff {
			if index+1 >= len(units) {
				zeroBytes(password)
				return nil, errInvalidPasswordText
			}
			next := rune(units[index+1])
			if next < 0xdc00 || next > 0xdfff {
				zeroBytes(password)
				return nil, errInvalidPasswordText
			}
			value = utf16.DecodeRune(value, next)
			index++
		} else if value >= 0xdc00 && value <= 0xdfff {
			zeroBytes(password)
			return nil, errInvalidPasswordText
		}
		size := utf8.RuneLen(value)
		if size < 0 || len(password)+size > maxVaultPasswordBytes {
			zeroBytes(password)
			return nil, errVaultPasswordTooLong
		}
		password = utf8.AppendRune(password, value)
	}
	return password, nil
}

func zeroUint16s(values []uint16) {
	for index := range values {
		values[index] = 0
	}
}

type windowsInputRecord struct {
	eventType uint16
	_         uint16
	event     [16]byte
}

type windowsKeyEventRecord struct {
	keyDown         int32
	repeat          uint16
	virtualKey      uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

// 対応するすべての Windows アーキテクチャで、INPUT_RECORD は 20 バイト、union の
// KEY_EVENT_RECORD は 16 バイトである。負のサイズ検査により、unsafe でデコードする
// 前提が崩れた場合はコンパイル時に失敗する。
var (
	_ [20 - unsafe.Sizeof(windowsInputRecord{})]byte
	_ [unsafe.Sizeof(windowsInputRecord{}) - 20]byte
	_ [16 - unsafe.Sizeof(windowsKeyEventRecord{})]byte
	_ [unsafe.Sizeof(windowsKeyEventRecord{}) - 16]byte
)

var readConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

func readWindowsConsoleInput(handle windows.Handle) (windowsKeyInput, error) {
	var record windowsInputRecord
	var read uint32
	succeeded, _, callErr := readConsoleInputW.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&read)),
	)
	if succeeded == 0 {
		if callErr == syscall.Errno(0) {
			callErr = errors.New("ReadConsoleInputW failed")
		}
		return windowsKeyInput{}, callErr
	}
	if read != 1 {
		return windowsKeyInput{}, io.EOF
	}
	if record.eventType != windows.KEY_EVENT {
		return windowsKeyInput{}, nil
	}
	event := (*windowsKeyEventRecord)(unsafe.Pointer(&record.event[0]))
	return windowsKeyInput{
		keyDown: event.keyDown != 0, repeat: event.repeat, virtualKey: event.virtualKey,
		unicodeChar: event.unicodeChar, controlKeyState: event.controlKeyState,
	}, nil
}

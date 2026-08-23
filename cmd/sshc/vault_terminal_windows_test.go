//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPasswordReaderCancellationWakesWaitAndRestoresExactMode(t *testing.T) {
	fake := newFakeWindowsPasswordOperations()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan windowsPasswordTestResult, 1)
	go func() {
		password, err := readWindowsPassword(ctx, windows.Handle(41), fake.operations())
		result <- windowsPasswordTestResult{password: password, err: err}
	}()
	// 待ちへ入ってから取り消す。ここで確かめたいのは「待っている reader を
	// 取り消しが起動する」ことである。mode が変わった時点で取り消すと、reader は
	// まだ待ちに入っておらず、ループの頭の ctx 検査で降りる。それも正しい
	// 振る舞いだが、起動されたことにはならないので、合図は一度も要らない。
	// 速い機械では reader が先に待ちへ入るので通り、混んだ CI で落ちていた。
	<-fake.waiting
	cancel()
	answer := receiveWindowsPasswordResult(t, result)
	if answer.password != nil || !errors.Is(answer.err, context.Canceled) {
		t.Fatalf("canceled read = %v, %v", answer.password, answer.err)
	}
	fake.assertRestored(t)
	if fake.setEventCalls != 1 || fake.closeEventCalls != 1 {
		t.Fatalf("cancel event calls: set=%d close=%d, want 1 each", fake.setEventCalls, fake.closeEventCalls)
	}
}

func TestWindowsPasswordReaderEditsUnicodeAndBoundsInput(t *testing.T) {
	tests := []struct {
		name      string
		events    []windowsKeyInput
		want      []byte
		wantError error
	}{
		{name: "return", events: windowsTextEvents([]uint16{'m', 'a', 's', 't', 'e', 'r', '\r'}), want: []byte("master")},
		{name: "backspace delete and control u", events: windowsTextEvents([]uint16{'x', 0x15, 'p', 0x00e4, 's', 's', '\b', 'd', 0x7f, '\r'}), want: []byte("p\u00e4s")},
		{name: "surrogate pair", events: windowsTextEvents([]uint16{0xd83d, 0xde03, '\r'}), want: []byte("\U0001f603")},
		{name: "backspace removes surrogate pair", events: append(windowsTextEvents([]uint16{0xd83d, 0xde03}), windowsKeyInput{keyDown: true, virtualKey: windows.VK_BACK}, windowsEnterKey()), want: nil},
		{name: "virtual delete removes the last rune", events: append(windowsTextEvents([]uint16{'o', 'k'}), windowsKeyInput{keyDown: true, virtualKey: windows.VK_DELETE}, windowsEnterKey()), want: []byte("o")},
		{name: "control d submits nonempty", events: windowsTextEvents([]uint16{'o', 'k', 0x04}), want: []byte("ok")},
		{name: "control d empty", events: windowsTextEvents([]uint16{0x04}), wantError: io.EOF},
		{name: "other editing controls are ignored", events: windowsTextEvents([]uint16{'a', 0x01, 0x09, 0x17, 'b', '\r'}), want: []byte("ab")},
		{name: "unpaired surrogate", events: windowsTextEvents([]uint16{0xd83d, '\r'}), wantError: errInvalidPasswordText},
		{name: "unpaired low surrogate", events: windowsTextEvents([]uint16{0xdc00, '\r'}), wantError: errInvalidPasswordText},
		{name: "maximum accepted", events: []windowsKeyInput{{keyDown: true, repeat: maxVaultPasswordBytes, unicodeChar: 'a'}, windowsEnterKey()}, want: bytes.Repeat([]byte{'a'}, maxVaultPasswordBytes)},
		{name: "one byte too many", events: []windowsKeyInput{{keyDown: true, repeat: maxVaultPasswordBytes + 1, unicodeChar: 'a'}}, wantError: errVaultPasswordTooLong},
		{name: "maximum multibyte accepted", events: []windowsKeyInput{{keyDown: true, repeat: 1365, unicodeChar: 0x20ac}, windowsEnterKey()}, want: bytes.Repeat([]byte{0xe2, 0x82, 0xac}, 1365)},
		{name: "multibyte exceeds byte limit", events: []windowsKeyInput{{keyDown: true, repeat: 1366, unicodeChar: 0x20ac}, windowsEnterKey()}, wantError: errVaultPasswordTooLong},
		{name: "control c cancels", events: windowsTextEvents([]uint16{'x', 0x03}), wantError: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeWindowsPasswordOperations()
			fake.events = append(fake.events, test.events...)
			close(fake.inputReady)
			password, err := readWindowsPassword(context.Background(), windows.Handle(42), fake.operations())
			defer zeroBytes(password)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
			if !bytes.Equal(password, test.want) {
				t.Fatalf("password = %v, want %v", password, test.want)
			}
			fake.assertRestored(t)
		})
	}
}

func TestWindowsPasswordReaderRestoresModeOnEventAndReadFailures(t *testing.T) {
	createFailure := errors.New("create event failed")
	fake := newFakeWindowsPasswordOperations()
	fake.createEventError = createFailure
	password, err := readWindowsPassword(context.Background(), windows.Handle(43), fake.operations())
	if password != nil || !errors.Is(err, createFailure) {
		t.Fatalf("create event failure = %v, %v", password, err)
	}
	fake.assertRestored(t)

	readFailure := errors.New("read input failed")
	fake = newFakeWindowsPasswordOperations()
	fake.readError = readFailure
	close(fake.inputReady)
	password, err = readWindowsPassword(context.Background(), windows.Handle(44), fake.operations())
	if password != nil || !errors.Is(err, readFailure) {
		t.Fatalf("read failure = %v, %v", password, err)
	}
	fake.assertRestored(t)

	waitFailure := errors.New("wait failed")
	fake = newFakeWindowsPasswordOperations()
	fake.waitError = waitFailure
	password, err = readWindowsPassword(context.Background(), windows.Handle(48), fake.operations())
	if password != nil || !errors.Is(err, waitFailure) {
		t.Fatalf("wait failure = %v, %v", password, err)
	}
	fake.assertRestored(t)
}

func TestWindowsPasswordReaderRestoreFailurePreservesCancellationAndZeroesResult(t *testing.T) {
	restoreFailure := errors.New("restore failed")
	fake := newFakeWindowsPasswordOperations()
	fake.events = windowsTextEvents([]uint16{0x03})
	fake.restoreError = restoreFailure
	close(fake.inputReady)
	password, err := readWindowsPassword(context.Background(), windows.Handle(47), fake.operations())
	if password != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, restoreFailure) {
		t.Fatalf("restore plus cancel = %v, %v", password, err)
	}
}

func TestWindowsPasswordEditingZeroesRemovedBackingUnits(t *testing.T) {
	units := make([]uint16, 0, 8)
	units = append(units, 's', 'e', 'c', 'r', 'e', 't')
	backing := units[:cap(units)]
	if _, err := consumeWindowsPasswordKey(&units, windowsKeyInput{keyDown: true, virtualKey: windows.VK_BACK}); err != nil {
		t.Fatal(err)
	}
	if backing[5] != 0 {
		t.Fatalf("backspace left removed unit %#x in backing storage", backing[5])
	}
	if _, err := consumeWindowsPasswordKey(&units, windowsKeyInput{keyDown: true, unicodeChar: 0x15}); err != nil {
		t.Fatal(err)
	}
	for index, value := range backing[:5] {
		if value != 0 {
			t.Fatalf("control-U left unit %d as %#x in backing storage", index, value)
		}
	}
}

func TestWindowsPasswordReaderAlreadyCanceledDoesNotTouchConsole(t *testing.T) {
	fake := newFakeWindowsPasswordOperations()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	password, err := readWindowsPassword(ctx, windows.Handle(45), fake.operations())
	if password != nil || !errors.Is(err, context.Canceled) || fake.getModeCalls != 0 || len(fake.setModes) != 0 {
		t.Fatalf("pre-cancel = %v, %v, get=%d set=%v", password, err, fake.getModeCalls, fake.setModes)
	}
}

func TestWindowsPasswordReaderModeSetupFailuresDoNotStartAWaiter(t *testing.T) {
	getFailure := errors.New("get mode failed")
	fake := newFakeWindowsPasswordOperations()
	fake.getModeError = getFailure
	password, err := readWindowsPassword(context.Background(), windows.Handle(49), fake.operations())
	if password != nil || !errors.Is(err, getFailure) || fake.getModeCalls != 1 || len(fake.setModes) != 0 {
		t.Fatalf("get mode failure = %v, %v, get=%d set=%v", password, err, fake.getModeCalls, fake.setModes)
	}

	rawFailure := errors.New("set raw mode failed")
	fake = newFakeWindowsPasswordOperations()
	fake.rawModeError = rawFailure
	password, err = readWindowsPassword(context.Background(), windows.Handle(50), fake.operations())
	if password != nil || !errors.Is(err, rawFailure) || fake.getModeCalls != 1 || len(fake.setModes) != 1 {
		t.Fatalf("set raw failure = %v, %v, get=%d set=%v", password, err, fake.getModeCalls, fake.setModes)
	}
}

func TestWindowsPasswordReaderReadCancelRaceReturnsAndRestoresMode(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		fake := newFakeWindowsPasswordOperations()
		fake.events = windowsTextEvents([]uint16{'o', 'k', '\r'})
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan windowsPasswordTestResult, 1)
		go func() {
			password, err := readWindowsPassword(ctx, windows.Handle(46), fake.operations())
			result <- windowsPasswordTestResult{password: password, err: err}
		}()
		<-fake.modeChanged
		start := make(chan struct{})
		var actions sync.WaitGroup
		actions.Add(2)
		go func() { defer actions.Done(); <-start; cancel() }()
		go func() { defer actions.Done(); <-start; close(fake.inputReady) }()
		close(start)
		actions.Wait()
		answer := receiveWindowsPasswordResult(t, result)
		if answer.err != nil && !errors.Is(answer.err, context.Canceled) {
			t.Fatalf("iteration %d: error = %v", iteration, answer.err)
		}
		if answer.err == nil && !bytes.Equal(answer.password, []byte("ok")) {
			t.Fatalf("iteration %d: password = %q", iteration, answer.password)
		}
		zeroBytes(answer.password)
		fake.assertRestored(t)
	}
}

type fakeWindowsPasswordOperations struct {
	mu              sync.Mutex
	oldMode         uint32
	modeChanged     chan struct{}
	modeChangedOnce sync.Once
	cancelEvent     chan struct{}
	cancelEventOnce sync.Once
	// waiting は、reader が実際に待ちへ入ったことを表す。mode が変わったこと
	// では代わりにならない。あれは待ちに入るより前に起きるので、そこで
	// 取り消すと reader は待たずにループの頭の ctx 検査で降りる。
	waiting          chan struct{}
	waitingOnce      sync.Once
	inputReady       chan struct{}
	events           []windowsKeyInput
	getModeCalls     int
	setModes         []uint32
	setEventCalls    int
	closeEventCalls  int
	createEventError error
	getModeError     error
	rawModeError     error
	readError        error
	waitError        error
	restoreError     error
}

type windowsPasswordTestResult struct {
	password []byte
	err      error
}

func newFakeWindowsPasswordOperations() *fakeWindowsPasswordOperations {
	return &fakeWindowsPasswordOperations{
		oldMode:     0xfedcba9f,
		modeChanged: make(chan struct{}),
		cancelEvent: make(chan struct{}),
		inputReady:  make(chan struct{}),
		waiting:     make(chan struct{}),
	}
}

func (f *fakeWindowsPasswordOperations) operations() windowsPasswordOperations {
	return windowsPasswordOperations{
		getMode: func(windows.Handle) (uint32, error) {
			f.getModeCalls++
			return f.oldMode, f.getModeError
		},
		setMode: func(_ windows.Handle, mode uint32) error {
			f.mu.Lock()
			f.setModes = append(f.setModes, mode)
			count := len(f.setModes)
			f.mu.Unlock()
			if count == 1 {
				f.modeChangedOnce.Do(func() { close(f.modeChanged) })
				return f.rawModeError
			}
			if count == 2 {
				return f.restoreError
			}
			return nil
		},
		createEvent: func() (windows.Handle, error) {
			return windows.Handle(91), f.createEventError
		},
		setEvent: func(windows.Handle) error {
			f.mu.Lock()
			f.setEventCalls++
			f.mu.Unlock()
			f.cancelEventOnce.Do(func() { close(f.cancelEvent) })
			return nil
		},
		closeHandle: func(windows.Handle) error {
			f.mu.Lock()
			f.closeEventCalls++
			f.mu.Unlock()
			return nil
		},
		wait: func([]windows.Handle) (uint32, error) {
			if f.waitError != nil {
				return windows.WAIT_FAILED, f.waitError
			}
			f.waitingOnce.Do(func() { close(f.waiting) })
			select {
			case <-f.cancelEvent:
				return windows.WAIT_OBJECT_0, nil
			case <-f.inputReady:
				return windows.WAIT_OBJECT_0 + 1, nil
			}
		},
		readInput: func(windows.Handle) (windowsKeyInput, error) {
			if f.readError != nil {
				return windowsKeyInput{}, f.readError
			}
			if len(f.events) == 0 {
				return windowsKeyInput{}, io.EOF
			}
			event := f.events[0]
			f.events = f.events[1:]
			return event, nil
		},
	}
}

func (f *fakeWindowsPasswordOperations) assertRestored(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.setModes) != 2 {
		t.Fatalf("set modes = %v, want raw and exact restore", f.setModes)
	}
	raw := f.setModes[0]
	if raw&(windows.ENABLE_ECHO_INPUT|windows.ENABLE_LINE_INPUT|windows.ENABLE_PROCESSED_INPUT) != 0 {
		t.Fatalf("raw mode left echo/line/processed input enabled: %#x", raw)
	}
	if f.setModes[1] != f.oldMode {
		t.Fatalf("restored mode = %#x, want exact %#x", f.setModes[1], f.oldMode)
	}
}

func windowsTextEvents(units []uint16) []windowsKeyInput {
	events := make([]windowsKeyInput, 0, len(units))
	for _, unit := range units {
		events = append(events, windowsKeyInput{keyDown: true, repeat: 1, virtualKey: unit, unicodeChar: unit})
	}
	return events
}

func windowsEnterKey() windowsKeyInput {
	return windowsKeyInput{keyDown: true, repeat: 1, virtualKey: windows.VK_RETURN, unicodeChar: '\r'}
}

func receiveWindowsPasswordResult(t *testing.T, result <-chan windowsPasswordTestResult) windowsPasswordTestResult {
	t.Helper()
	select {
	case answer := <-result:
		return answer
	case <-time.After(2 * time.Second):
		t.Fatal("Windows password reader did not return")
		return windowsPasswordTestResult{}
	}
}

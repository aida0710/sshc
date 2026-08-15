//go:build unix

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"sshc/internal/handoff"
)

const (
	vaultPTYHelperEnvironment = "SSHC_VAULT_PTY_HELPER"
	vaultPTYStateEnvironment  = "SSHC_VAULT_PTY_STATE"
)

func TestRunVaultPromptCtrlCReturns130WithoutEnterAndRestoresEcho(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerHeadless, false, false))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerHeadless)

	command := exec.Command(os.Args[0], "-test.run=^TestVaultPromptPTYHelperProcess$")
	command.Env = append(os.Environ(), vaultPTYHelperEnvironment+"=1", vaultPTYStateEnvironment+"="+stateDir)
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = terminal.Close()
	}()

	readPTYThrough(t, terminal, []byte("New master password: "), 2*time.Second)
	if _, err := terminal.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	// This marker is printed only after runVault has returned 130. Observing it
	// before any Enter byte proves Ctrl-C released the password prompt itself.
	readPTYThrough(t, terminal, []byte("vault-helper-returned-130"), 2*time.Second)

	// The helper is now reading a normal canonical line. Input must be echoed
	// immediately, before its terminating Enter, if the exact prior mode returned.
	const echoProbe = "vault-echo-restored-probe"
	if _, err := terminal.Write([]byte(echoProbe)); err != nil {
		t.Fatal(err)
	}
	readPTYThrough(t, terminal, []byte(echoProbe), 2*time.Second)
	if _, err := terminal.Write([]byte{'\n'}); err != nil {
		t.Fatal(err)
	}

	err = command.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 130 {
		t.Fatalf("helper exit = %v, want 130", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want only the status preflight and no mutation request", got)
	}
}

func TestVaultPromptPTYHelperProcess(t *testing.T) {
	if os.Getenv(vaultPTYHelperEnvironment) != "1" {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	code := runVault(ctx, "create", os.Getenv(vaultPTYStateEnvironment), &http.Client{Timeout: 3 * time.Minute},
		os.Stdin, os.Stdout, os.Stderr, systemPasswordTerminal{})
	if code == 130 {
		fmt.Fprintln(os.Stdout, "vault-helper-returned-130")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code)
}

type unixPasswordTestResult struct {
	password []byte
	err      error
}

func TestUnixPasswordReaderEditsAndBoundsBytesWithoutControlCharacters(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		want      []byte
		wantError error
	}{
		{name: "line feed", input: []byte("master\n"), want: []byte("master")},
		{name: "carriage return", input: []byte("master\r"), want: []byte("master")},
		{name: "backspace and delete", input: append([]byte("p\u00e4ss"), '\b', 'd', 0x7f, '\n'), want: []byte("p\u00e4s")},
		{name: "control u clears and zeroes line", input: append([]byte("discard"), 0x15, 'k', 'e', 'e', 'p', '\n'), want: []byte("keep")},
		{name: "other editing controls are ignored", input: []byte{'a', 0x01, 0x09, 0x17, 'b', '\n'}, want: []byte("ab")},
		{name: "control d submits a nonempty line", input: append([]byte("master"), 0x04), want: []byte("master")},
		{name: "control d on empty is eof", input: []byte{0x04}, wantError: io.EOF},
		{name: "invalid utf8 is rejected", input: []byte{0xff, '\n'}, wantError: errInvalidPasswordText},
		{name: "maximum accepted", input: append(bytes.Repeat([]byte{'a'}, maxVaultPasswordBytes), '\n'), want: bytes.Repeat([]byte{'a'}, maxVaultPasswordBytes)},
		{name: "one byte too many", input: bytes.Repeat([]byte{'a'}, maxVaultPasswordBytes+1), wantError: errVaultPasswordTooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operations, restored := scriptedUnixPasswordOperations(test.input, io.EOF)
			password, err := readUnixPassword(context.Background(), vaultTestInput(t), operations)
			defer zeroBytes(password)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
			if !bytes.Equal(password, test.want) {
				t.Fatalf("password bytes = %v, want %v", password, test.want)
			}
			if *restored != 1 {
				t.Fatalf("restore calls = %d, want 1", *restored)
			}
		})
	}
}

func TestUnixPasswordReaderRestoresSavedModeOnSetupAndReadFailures(t *testing.T) {
	setupFailure := errors.New("pipe failed")
	saved := new(term.State)
	restored := 0
	operations := unixPasswordOperations{
		makeRaw: func(int) (*term.State, error) { return saved, nil },
		restore: func(_ int, state *term.State) error {
			if state != saved {
				t.Fatal("restore did not receive the exact saved state")
			}
			restored++
			return nil
		},
		pipe: func() (*os.File, *os.File, error) { return nil, nil, setupFailure },
	}
	if password, err := readUnixPassword(context.Background(), vaultTestInput(t), operations); password != nil || !errors.Is(err, setupFailure) {
		t.Fatalf("pipe failure = %v, %v", password, err)
	}
	if restored != 1 {
		t.Fatalf("restore calls after pipe failure = %d, want 1", restored)
	}

	readFailure := errors.New("read failed")
	operations, restoredPointer := scriptedUnixPasswordOperations([]byte("partial"), readFailure)
	password, err := readUnixPassword(context.Background(), vaultTestInput(t), operations)
	if password != nil || !errors.Is(err, readFailure) {
		t.Fatalf("read failure = %v, %v", password, err)
	}
	if *restoredPointer != 1 {
		t.Fatalf("restore calls after read failure = %d, want 1", *restoredPointer)
	}

	pollFailure := errors.New("poll failed")
	operations, restoredPointer = scriptedUnixPasswordOperations(nil, io.EOF)
	operations.poll = func([]unix.PollFd, int) (int, error) { return 0, pollFailure }
	password, err = readUnixPassword(context.Background(), vaultTestInput(t), operations)
	if password != nil || !errors.Is(err, pollFailure) {
		t.Fatalf("poll failure = %v, %v", password, err)
	}
	if *restoredPointer != 1 {
		t.Fatalf("restore calls after poll failure = %d, want 1", *restoredPointer)
	}
}

func TestUnixPasswordReaderRestoreFailurePreservesCancellationAndZeroesResult(t *testing.T) {
	restoreFailure := errors.New("restore failed")
	operations, _ := scriptedUnixPasswordOperations([]byte{0x03}, io.EOF)
	operations.restore = func(int, *term.State) error { return restoreFailure }
	password, err := readUnixPassword(context.Background(), vaultTestInput(t), operations)
	if password != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, restoreFailure) {
		t.Fatalf("restore plus cancel = %v, %v", password, err)
	}
}

func TestUnixPasswordEditingZeroesRemovedBackingBytes(t *testing.T) {
	password := make([]byte, 0, 16)
	password = append(password, []byte("secret")...)
	backing := password[:cap(password)]
	if _, err := consumeUnixPasswordByte(&password, '\b'); err != nil {
		t.Fatal(err)
	}
	if backing[5] != 0 {
		t.Fatalf("backspace left removed byte %#x in backing storage", backing[5])
	}
	if _, err := consumeUnixPasswordByte(&password, 0x15); err != nil {
		t.Fatal(err)
	}
	for index, value := range backing[:5] {
		if value != 0 {
			t.Fatalf("control-U left byte %d as %#x in backing storage", index, value)
		}
	}
}

func TestUnixPasswordReaderDoesNotChangeModeWhenAlreadyCanceledOrMakeRawFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	operations := unixPasswordOperations{
		makeRaw: func(int) (*term.State, error) { calls++; return nil, errors.New("must not be called") },
	}
	password, err := readUnixPassword(ctx, vaultTestInput(t), operations)
	if password != nil || !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-cancel = %v, %v, makeRaw calls %d", password, err, calls)
	}

	makeRawFailure := errors.New("make raw failed")
	restoreCalls := 0
	operations = unixPasswordOperations{
		makeRaw: func(int) (*term.State, error) { return nil, makeRawFailure },
		restore: func(int, *term.State) error { restoreCalls++; return nil },
	}
	password, err = readUnixPassword(context.Background(), vaultTestInput(t), operations)
	if password != nil || !errors.Is(err, makeRawFailure) || restoreCalls != 0 {
		t.Fatalf("makeRaw failure = %v, %v, restore calls %d", password, err, restoreCalls)
	}
}

func TestUnixPasswordReaderCancellationDuringReadRestoresExactMode(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	prior, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan unixPasswordTestResult, 1)
	go func() {
		password, err := systemPasswordTerminal{}.ReadPassword(ctx, slave)
		result <- unixPasswordTestResult{password: password, err: err}
	}()
	waitForTerminalModeChange(t, slave, prior)
	cancel()
	answer := receiveUnixPasswordResult(t, result)
	defer zeroBytes(answer.password)
	if answer.password != nil || !errors.Is(answer.err, context.Canceled) {
		t.Fatalf("canceled read = %v, %v", answer.password, answer.err)
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prior, after) {
		t.Fatal("terminal mode was not restored exactly after context cancellation")
	}
}

func TestUnixPasswordReaderSuccessRestoresExactMode(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	prior, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan unixPasswordTestResult, 1)
	go func() {
		password, err := systemPasswordTerminal{}.ReadPassword(context.Background(), slave)
		result <- unixPasswordTestResult{password: password, err: err}
	}()
	waitForTerminalModeChange(t, slave, prior)
	if _, err := master.Write([]byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	answer := receiveUnixPasswordResult(t, result)
	defer zeroBytes(answer.password)
	if answer.err != nil || !bytes.Equal(answer.password, []byte("ok")) {
		t.Fatalf("successful read = %q, %v", answer.password, answer.err)
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil || !reflect.DeepEqual(prior, after) {
		t.Fatalf("terminal mode not restored after success: %v", err)
	}
}

func TestUnixPasswordReaderReadCancelRaceAlwaysRestoresModeAndReturns(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		master, slave, err := pty.Open()
		if err != nil {
			t.Fatal(err)
		}
		prior, err := term.GetState(int(slave.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan unixPasswordTestResult, 1)
		go func() {
			password, err := systemPasswordTerminal{}.ReadPassword(ctx, slave)
			result <- unixPasswordTestResult{password: password, err: err}
		}()
		waitForTerminalModeChange(t, slave, prior)
		start := make(chan struct{})
		var actions sync.WaitGroup
		actions.Add(2)
		go func() { defer actions.Done(); <-start; cancel() }()
		go func() { defer actions.Done(); <-start; _, _ = master.Write([]byte("ok\n")) }()
		close(start)
		actions.Wait()
		answer := receiveUnixPasswordResult(t, result)
		if answer.err != nil && !errors.Is(answer.err, context.Canceled) {
			t.Fatalf("iteration %d: error = %v", iteration, answer.err)
		}
		if answer.err == nil && !bytes.Equal(answer.password, []byte("ok")) {
			t.Fatalf("iteration %d: password = %q", iteration, answer.password)
		}
		zeroBytes(answer.password)
		after, stateErr := term.GetState(int(slave.Fd()))
		if stateErr != nil || !reflect.DeepEqual(prior, after) {
			t.Fatalf("iteration %d: terminal mode not restored: %v", iteration, stateErr)
		}
		_ = master.Close()
		_ = slave.Close()
	}
}

func scriptedUnixPasswordOperations(input []byte, finalError error) (unixPasswordOperations, *int) {
	saved := new(term.State)
	restored := new(int)
	offset := 0
	return unixPasswordOperations{
		makeRaw: func(int) (*term.State, error) { return saved, nil },
		restore: func(_ int, state *term.State) error {
			if state != saved {
				return errors.New("wrong terminal state")
			}
			*restored++
			return nil
		},
		pipe: os.Pipe,
		poll: func(descriptors []unix.PollFd, timeout int) (int, error) {
			descriptors[1].Revents = unix.POLLIN
			return 1, nil
		},
		read: func(_ int, destination []byte) (int, error) {
			if offset >= len(input) {
				return 0, finalError
			}
			destination[0] = input[offset]
			offset++
			return 1, nil
		},
	}, restored
}

func waitForTerminalModeChange(t *testing.T, terminal *os.File, prior *term.State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		current, err := term.GetState(int(terminal.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(prior, current) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal mode did not change before read")
		}
		runtime.Gosched()
	}
}

func receiveUnixPasswordResult(t *testing.T, result <-chan unixPasswordTestResult) unixPasswordTestResult {
	t.Helper()
	select {
	case answer := <-result:
		return answer
	case <-time.After(2 * time.Second):
		t.Fatal("password reader did not return")
		return unixPasswordTestResult{}
	}
}

func TestReadPTYThroughRetriesInterruptedPoll(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	if _, err := writer.Write([]byte("ready")); err != nil {
		t.Fatal(err)
	}

	interrupted := false
	readPTYThroughWithPoll(t, reader, []byte("ready"), 2*time.Second,
		func(descriptors []unix.PollFd, timeout int) (int, error) {
			if !interrupted {
				interrupted = true
				return 0, unix.EINTR
			}
			return unix.Poll(descriptors, timeout)
		})
}

func readPTYThrough(t *testing.T, terminal *os.File, marker []byte, timeout time.Duration) {
	t.Helper()
	readPTYThroughWithPoll(t, terminal, marker, timeout, unix.Poll)
}

type vaultPTYPoll func([]unix.PollFd, int) (int, error)

func readPTYThroughWithPoll(
	t *testing.T, terminal *os.File, marker []byte, timeout time.Duration, poll vaultPTYPoll,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var received []byte
	buffer := make([]byte, 128)
	for !bytes.Contains(received, marker) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out reading PTY through %q; received %q", marker, received)
		}
		milliseconds := int(remaining / time.Millisecond)
		if milliseconds < 1 {
			milliseconds = 1
		}
		ready := []unix.PollFd{{Fd: int32(terminal.Fd()), Events: unix.POLLIN}}
		count, err := poll(ready, milliseconds)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			t.Fatalf("poll PTY through %q: %v; received %q", marker, err, received)
		}
		if count == 0 {
			t.Fatalf("timed out reading PTY through %q; received %q", marker, received)
		}
		read, err := terminal.Read(buffer)
		if read > 0 {
			received = append(received, buffer[:read]...)
		}
		if err != nil {
			t.Fatalf("read PTY through %q: %v; received %q", marker, err, received)
		}
	}
}

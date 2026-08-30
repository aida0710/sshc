//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// This test deliberately does not inject transportDependencies. A Linux PTY
// is not an electrical UART, but opening its slave exercises the production
// production serial backend and termios setup before the run/expect path sees it.
func TestRunSerialPTYEndToEnd(t *testing.T) {
	master, keepSlaveOpen, device := openSerialPTY(t)
	routerResult := make(chan virtualSerialResult, 1)
	routerDone := make(chan struct{})
	go func() {
		defer close(routerDone)
		serveVirtualSerialRouter(master, routerResult)
	}()
	defer func() {
		_ = keepSlaveOpen.Close()
		_ = master.Close()
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-routerDone:
		case <-timer.C:
			t.Error("virtual serial router did not stop during cleanup")
		}
	}()

	// An old prompt may already be buffered when automation starts. The real
	// serial Stream must discard it rather than accepting it as this command's
	// result.
	if _, err := master.WriteString("stale Router# "); err != nil {
		t.Fatalf("seed stale serial input: %v", err)
	}

	parsed, err := parseInvocation([]string{
		"sshc", "serial", device, "--non-interactive",
		"--expect", `Router# `,
		"--timeout", "3s",
		"--settle", "20ms",
		"--json",
		"--", "show", "version",
	})
	if err != nil {
		t.Fatalf("parse non-interactive serial invocation: %v", err)
	}
	if parsed.Kind != invocationRunTransport || parsed.Transport == nil {
		t.Fatalf("parsed invocation = %#v", parsed)
	}

	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open null stdin: %v", err)
	}
	defer stdin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := runTransportInvocation(ctx, *parsed.Transport, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run serial code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}

	select {
	case observed := <-routerResult:
		if observed.err != nil {
			t.Fatalf("virtual serial router: %v", observed.err)
		}
		if observed.command != "show version" || observed.terminator != '\r' {
			t.Fatalf("router received command %q terminated by %#x", observed.command, observed.terminator)
		}
	case <-ctx.Done():
		t.Fatal("virtual serial router did not receive the command before the test deadline")
	}

	var report transportRunReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode run serial report: %v\n%s", err, stdout.String())
	}
	if !report.Success || !report.Matched || report.Transport != transportSerial || report.Target != device {
		t.Fatalf("run serial report = %#v", report)
	}
	if strings.Contains(report.Transcript, "stale") {
		t.Fatalf("stale prompt was accepted as command output: %q", report.Transcript)
	}
	for _, want := range []string{"Cisco IOS Software", "Virtual router uptime is 1 day", "Router# "} {
		if !strings.Contains(report.Transcript, want) {
			t.Errorf("transcript does not contain %q: %q", want, report.Transcript)
		}
	}
}

func openSerialPTY(t *testing.T) (master, keepSlaveOpen *os.File, device string) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot open /dev/ptmx: %v", err)
	}
	fail := func(format string, args ...any) {
		_ = master.Close()
		t.Skipf(format, args...)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		fail("cannot unlock PTY: %v", err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		fail("cannot get PTY number: %v", err)
	}
	if err := makeSerialPTYRaw(master); err != nil {
		fail("cannot configure PTY raw mode: %v", err)
	}
	device = fmt.Sprintf("/dev/pts/%d", number)

	// Keep one slave descriptor open until the test finishes. Without it,
	// Linux may return EIO to the router's master-side Read in the short window
	// before the production serial backend opens the slave.
	keepSlaveOpen, err = os.OpenFile(device, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		fail("cannot open PTY slave: %v", err)
	}
	return master, keepSlaveOpen, device
}

func makeSerialPTYRaw(file *os.File) error {
	termios, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	raw := *termios
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	return unix.IoctlSetTermios(int(file.Fd()), unix.TCSETS, &raw)
}

type virtualSerialResult struct {
	command    string
	terminator byte
	err        error
}

func serveVirtualSerialRouter(master *os.File, result chan<- virtualSerialResult) {
	buffer := make([]byte, 256)
	line := make([]byte, 0, 64)
	for {
		count, err := master.Read(buffer)
		if err != nil {
			result <- virtualSerialResult{err: fmt.Errorf("read command: %w", err)}
			return
		}
		if count == 0 {
			result <- virtualSerialResult{err: errors.New("command read made no progress")}
			return
		}
		for _, character := range buffer[:count] {
			if character != '\r' && character != '\n' {
				line = append(line, character)
				continue
			}
			if character != '\r' {
				result <- virtualSerialResult{command: string(line), terminator: character, err: errors.New("serial command used LF instead of CR")}
				return
			}
			response := "\r\nCisco IOS Software, virtual image\r\n" +
				"Virtual router uptime is 1 day\r\nRouter# "
			if _, err := master.WriteString(response); err != nil {
				result <- virtualSerialResult{command: string(line), terminator: character, err: fmt.Errorf("write response: %w", err)}
				return
			}
			result <- virtualSerialResult{command: string(line), terminator: character}
			return
		}
	}
}

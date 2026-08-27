//go:build linux

package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestBuiltBinaryRunsAgainstVirtualSerialRouter opens a real PTY slave through
// the production serial driver. It complements the in-process test by proving
// main, argv, stdout JSON, and the executable's exit status together.
func TestBuiltBinaryRunsAgainstVirtualSerialRouter(t *testing.T) {
	master, slave, device := openIntegrationSerialPTY(t)
	routerDone := make(chan integrationSerialResult, 1)
	go serveIntegrationSerialRouter(master, routerDone)

	if _, err := master.WriteString("stale virtual# "); err != nil {
		t.Fatal(err)
	}
	process := start(t, isolatedHome(t),
		"run", "serial", device,
		"--timeout", "3s", "--settle", "20ms", "--expect", `virtual# `,
		"--json", "--", "show", "version",
	)
	if code := process.wait(t, 5*time.Second); code != 0 {
		t.Fatalf("sshc run serial exit = %d\nstdout: %s\nstderr: %s", code, process.Stdout.String(), process.Stderr.String())
	}
	if process.Stderr.String() != "" {
		t.Fatalf("sshc run serial stderr = %q", process.Stderr.String())
	}
	select {
	case result := <-routerDone:
		if result.err != nil || result.command != "show version" || result.terminator != '\r' {
			t.Fatalf("virtual serial command = %q terminator=%#x error=%v", result.command, result.terminator, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("virtual serial router did not receive the command")
	}
	var report struct {
		Transport  string `json:"transport"`
		Target     string `json:"target"`
		Success    bool   `json:"success"`
		Matched    bool   `json:"matched"`
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal([]byte(process.Stdout.String()), &report); err != nil {
		t.Fatalf("decode sshc run serial JSON: %v\n%s", err, process.Stdout.String())
	}
	if report.Transport != "serial" || report.Target != device || !report.Success || !report.Matched {
		t.Fatalf("sshc run serial report = %#v", report)
	}
	if strings.Contains(report.Transcript, "stale") || !strings.Contains(report.Transcript, "virtual serial version 1") {
		t.Fatalf("sshc run serial transcript = %q", report.Transcript)
	}

	// The keep-open slave prevents master-side EIO before sshc opens the device.
	// It is not the transport under test and is closed only after sshc exits.
	_ = slave.Close()
}

func openIntegrationSerialPTY(t *testing.T) (master, slave *os.File, device string) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("cannot unlock PTY: %v", err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Skipf("cannot get PTY number: %v", err)
	}
	termios, err := unix.IoctlGetTermios(int(master.Fd()), unix.TCGETS)
	if err != nil {
		t.Skipf("cannot read PTY termios: %v", err)
	}
	raw := *termios
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	if err := unix.IoctlSetTermios(int(master.Fd()), unix.TCSETS, &raw); err != nil {
		t.Skipf("cannot configure PTY raw mode: %v", err)
	}
	device = fmt.Sprintf("/dev/pts/%d", number)
	slave, err = os.OpenFile(device, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot keep PTY slave open: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave, device
}

type integrationSerialResult struct {
	command    string
	terminator byte
	err        error
}

func serveIntegrationSerialRouter(master *os.File, result chan<- integrationSerialResult) {
	buffer := make([]byte, 256)
	var line bytes.Buffer
	for {
		count, err := master.Read(buffer)
		if err != nil {
			result <- integrationSerialResult{err: fmt.Errorf("read serial command: %w", err)}
			return
		}
		if count == 0 {
			result <- integrationSerialResult{err: errors.New("serial command read made no progress")}
			return
		}
		for _, value := range buffer[:count] {
			if value != '\r' && value != '\n' {
				_ = line.WriteByte(value)
				continue
			}
			if _, err := master.WriteString("\r\nvirtual serial version 1\r\nvirtual# "); err != nil {
				result <- integrationSerialResult{command: line.String(), terminator: value, err: err}
				return
			}
			result <- integrationSerialResult{command: line.String(), terminator: value}
			return
		}
	}
}

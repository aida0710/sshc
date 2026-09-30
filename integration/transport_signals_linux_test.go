//go:build linux

package integration

import (
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// synSentState は、/proc/net/tcp の st の欄で TCP_SYN_SENT を表す値。
const synSentState = "02"

// listenWithFullAcceptQueue は、accept しない listener を作り、受け入れの待ち行列を
// 1 本の接続で埋めて、その port を返す。Linux は待ち行列が埋まった listener への SYN を
// 捨てるので、次に来る接続は SYN を送り直しながら connect で待ち続ける。
func listenWithFullAcceptQueue(t *testing.T) int {
	t.Helper()
	listener, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Close(listener) })
	if err := syscall.Bind(listener, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	// backlog 0 の待ち行列には 1 本だけ入る。
	if err := syscall.Listen(listener, 0); err != nil {
		t.Fatal(err)
	}
	bound, err := syscall.Getsockname(listener)
	if err != nil {
		t.Fatal(err)
	}
	port := bound.(*syscall.SockaddrInet4).Port
	filler, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filler.Close() })
	return port
}

// connectingTo は、port へ SYN を送って応答を待っている接続が /proc/net/tcp にあるかを返す。
func connectingTo(t *testing.T, port int) bool {
	t.Helper()
	table, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(table), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		_, remotePortHex, _ := strings.Cut(fields[2], ":")
		remotePort, err := strconv.ParseUint(remotePortHex, 16, 16)
		if err == nil && int(remotePort) == port && fields[3] == synSentState {
			return true
		}
	}
	return false
}

// 接続を待っている間に止められても、接続の失敗（1）ではなく、止められたものとして
// 130 と kind=interrupted で終わる。
func TestAnAutomatedTelnetRunStoppedWhileConnectingReportsAnInterruption(t *testing.T) {
	for _, test := range telnetStopSignals {
		t.Run(test.name, func(t *testing.T) {
			port := listenWithFullAcceptQueue(t)

			run := start(t, isolatedHome(t),
				"telnet", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "--non-interactive",
				"--connect-timeout", "30s", "--timeout", "30s",
				"--expect", `never-sent# $`, "--json", "--", "show", "version",
			)
			deadline := time.Now().Add(automatedTelnetReadyTimeout)
			for !connectingTo(t, port) {
				if !run.running() || time.Now().After(deadline) {
					t.Fatalf("sshc telnet did not start connecting\n%s", run.Stderr.String())
				}
				time.Sleep(10 * time.Millisecond)
			}

			if err := run.Command.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}

			if code := run.wait(t, stoppedTelnetExitTimeout); code != 130 {
				t.Errorf("exit = %d, want 130\n%s", code, run.Stderr.String())
			}
			var report struct {
				Failure struct {
					Kind string `json:"kind"`
				} `json:"failure"`
			}
			if err := json.Unmarshal([]byte(run.Stdout.String()), &report); err != nil {
				t.Fatalf("stdout is not a JSON report: %v\n%s", err, run.Stdout.String())
			}
			if report.Failure.Kind != "interrupted" {
				t.Errorf("failure kind = %q, want interrupted\n%s", report.Failure.Kind, run.Stdout.String())
			}
		})
	}
}

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// unreachableEngineCommand は、engine に届かないときの案内を確かめるコマンドの呼び方。
type unreachableEngineCommand func(ctx context.Context, stateDir string, client *http.Client, stderr io.Writer) int

// sshc open、sshc status、sshc vault は、sshc ssh と同じく、engine に届かない理由
// ごとに利用者が次にすることを案内する。
var unreachableEngineCommands = map[string]unreachableEngineCommand{
	"open": func(ctx context.Context, stateDir string, client *http.Client, stderr io.Writer) int {
		return runOpen(ctx, commandEnvironment{stateDir: stateDir, client: client, stdout: io.Discard, stderr: stderr}, false)
	},
	"status": func(ctx context.Context, stateDir string, client *http.Client, stderr io.Writer) int {
		return runStatus(ctx, commandEnvironment{stateDir: stateDir, client: client, stdout: io.Discard, stderr: stderr}, false)
	},
	"vault status": func(ctx context.Context, stateDir string, client *http.Client, stderr io.Writer) int {
		return runVault(ctx, "status", commandEnvironment{
			stateDir: stateDir, client: client, stdout: io.Discard, stderr: stderr,
		})
	},
}

// handoffToListener は、handoff が address を指す状態を作る。
func handoffToListener(t *testing.T, address net.Addr) string {
	t.Helper()
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, "http://"+address.String())
	return stateDir
}

// silentListener は、接続を受け付けるが応答しない process の代わり。
func silentListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func TestAStaleHandoffWithNothingListeningIsReportedAsNotRunning(t *testing.T) {
	for name, run := range unreachableEngineCommands {
		t.Run(name, func(t *testing.T) {
			closed, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			stateDir := handoffToListener(t, closed.Addr())
			_ = closed.Close()
			var stderr strings.Builder

			code := run(context.Background(), stateDir, &http.Client{}, &stderr)

			if code != 1 || !strings.Contains(stderr.String(), "sshc is not running; run sshc engine") {
				t.Fatalf("exit = %d, stderr = %q; want 1 and the advice to run sshc engine", code, stderr.String())
			}
		})
	}
}

func TestAnEngineAddressThatNeverAnswersIsReportedAsNotResponding(t *testing.T) {
	for name, run := range unreachableEngineCommands {
		t.Run(name, func(t *testing.T) {
			stateDir := handoffToListener(t, silentListener(t).Addr())
			var stderr strings.Builder

			code := run(context.Background(), stateDir, &http.Client{Timeout: silentEngineClientTimeout}, &stderr)

			if code != 1 || !strings.Contains(stderr.String(), errEngineNotResponding.Error()) {
				t.Fatalf("exit = %d, stderr = %q; want 1 and %q", code, stderr.String(), errEngineNotResponding)
			}
		})
	}
}

func TestCtrlCWhileCheckingTheEngineEndsWith130AndNoAdvice(t *testing.T) {
	for name, run := range unreachableEngineCommands {
		t.Run(name, func(t *testing.T) {
			listener := silentListener(t)
			stateDir := handoffToListener(t, listener.Addr())
			ctx, interrupt := context.WithCancel(context.Background())
			defer interrupt()
			go func() {
				accepted, err := listener.Accept()
				if err != nil {
					return
				}
				defer func() { _ = accepted.Close() }()
				interrupt()
				<-ctx.Done()
			}()
			var stderr strings.Builder

			code := run(ctx, stateDir, &http.Client{}, &stderr)

			if code != 130 || stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %q; want 130 and nothing", code, stderr.String())
			}
		})
	}
}

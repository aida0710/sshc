//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"sshc/internal/app"
	"sshc/internal/handoff"
)

const (
	interactiveSignalHelperEnvironment = "SSHC_INTERACTIVE_SIGNAL_HELPER"
	interactiveSignalHomeEnvironment   = "SSHC_INTERACTIVE_SIGNAL_HOME"
	// interactiveSignalHelperReturned は、helper の dispatchInvocation が戻った後に
	// だけ出る。続く数字が終了コードである。
	interactiveSignalHelperReturned = "interactive-signal-helper-returned-"
	// pickerRestoreSequence は、選択画面がターミナルを戻すときに書く列である。
	// カーソルを表示し、代替画面を抜ける。
	pickerRestoreSequence = "\x1b[?25h\x1b[?1049l"
	// interactiveSignalPTYTimeout は、PTY の向こうの表示を待つ上限である。
	// 手元では数十ミリ秒で届くので、遅い CI でも誤って落ちない長さにする。
	interactiveSignalPTYTimeout = 5 * time.Second
)

// writePickerHome は、選択画面に alias を 1 つだけ並べるホームを作る。
func writePickerHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssh, "config"), []byte("Host alpha\n  HostName alpha.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

// os.Stdin の Read は ctx では戻らない。読み取りを待ったまま止められても、
// 代替画面とカーソルの非表示、raw のモードを残してはならない。
func TestTUIPickerRestoresTheTerminalWhenItsContextStops(t *testing.T) {
	terminal, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer tty.Close()
	before, err := term.GetState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}

	home := writePickerHome(t)
	ctx, stop := context.WithCancelCause(context.Background())
	defer stop(nil)
	returned := make(chan error, 1)
	go func() {
		_, err := chooseTUIHost(ctx, tuiPicker{home: home, input: tty, output: tty, stderr: io.Discard})
		returned <- err
	}()
	readPTYThrough(t, terminal, []byte("Enter connect"), interactiveSignalPTYTimeout)
	stop(errTerminated)

	select {
	case err := <-returned:
		if !errors.Is(err, errTerminated) {
			t.Fatalf("chooseTUIHost = %v, want the stop cause", err)
		}
	case <-time.After(interactiveSignalPTYTimeout):
		t.Fatal("the picker kept waiting for input after its context stopped")
	}
	readPTYThrough(t, terminal, []byte(pickerRestoreSequence), interactiveSignalPTYTimeout)
	after, err := term.GetState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("the picker left the terminal in raw mode")
	}
}

// `kill` で止めた `sshc ssh` が、代替画面と非表示のカーソル、raw のモードを
// 残すと、利用者は reset を打つまで scrollback もカーソルも見えない。
func TestSSHPickerRestoresTheTerminalOnSIGTERM(t *testing.T) {
	command, terminal := startInteractiveSignalHelper(t, "choose", writePickerHome(t))
	readPTYThrough(t, terminal, []byte("Enter connect"), interactiveSignalPTYTimeout)
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// 戻す列と helper の戻りの行は続けて書かれ、1 回の読み取りでまとめて届くことが
	// ある。戻りの行まで読んでから、戻す列がその前にあることを確かめる。
	// 監督者の SIGTERM は失敗ではない。engine と Serial／Telnet の対話接続と同じく 0。
	returned := []byte(interactiveSignalHelperReturned + "0")
	received := readPTYThrough(t, terminal, returned, interactiveSignalPTYTimeout)
	restoredAt := bytes.Index(received, []byte(pickerRestoreSequence))
	if restoredAt < 0 || restoredAt > bytes.Index(received, returned) {
		t.Fatalf("the picker did not restore the terminal before sshc returned; received %q", received)
	}
	expectEchoRestored(t, terminal)
}

// Vault のパスワード入力はエコーを止める。SIGTERM でも、戻す defer を通って
// から 130 で終える。
func TestVaultPromptRestoresEchoOnSIGTERM(t *testing.T) {
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(response, vaultStatusBody(handoff.OwnerEngine, false, false))
	}))
	defer server.Close()
	home := t.TempDir()
	stateDir := app.HandoffDir(home)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeVaultTestHandoff(t, stateDir, server.URL, handoff.OwnerEngine)

	command, terminal := startInteractiveSignalHelper(t, "vault-create", home)
	readPTYThrough(t, terminal, []byte("New master password: "), interactiveSignalPTYTimeout)
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	readPTYThrough(t, terminal, []byte(interactiveSignalHelperReturned+"130"), interactiveSignalPTYTimeout)
	expectEchoRestored(t, terminal)
}

// startInteractiveSignalHelper は、このテストバイナリを PTY の中で起動し、
// TestInteractiveSignalHelperProcess に command の呼び出しを dispatchInvocation
// から走らせる。HOME も一時ディレクトリに向け、利用者のホームには書かない。
func startInteractiveSignalHelper(t *testing.T, command, home string) (*exec.Cmd, *os.File) {
	t.Helper()
	helper := exec.Command(os.Args[0], "-test.run=^TestInteractiveSignalHelperProcess$")
	helper.Env = append(os.Environ(),
		interactiveSignalHelperEnvironment+"="+command,
		interactiveSignalHomeEnvironment+"="+home,
		"HOME="+home)
	terminal, err := pty.Start(helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = helper.Process.Kill()
		_ = helper.Wait()
		_ = terminal.Close()
	})
	return helper, terminal
}

// expectEchoRestored は、ターミナルが元の cooked のモードに戻り、打った文字が
// 読み手を待たずにエコーされることを確かめる。
func expectEchoRestored(t *testing.T, terminal *os.File) {
	t.Helper()
	const echoProbe = "interactive-echo-restored-probe"
	if _, err := terminal.Write([]byte(echoProbe)); err != nil {
		t.Fatal(err)
	}
	readPTYThrough(t, terminal, []byte(echoProbe), interactiveSignalPTYTimeout)
}

func TestInteractiveSignalHelperProcess(t *testing.T) {
	command := os.Getenv(interactiveSignalHelperEnvironment)
	if command == "" {
		return
	}
	called := invocation{Kind: invocationChoose}
	if command == "vault-create" {
		called = invocation{Kind: invocationVault, Args: []string{"create"}}
	}
	code := dispatchInvocation(called, os.Getenv(interactiveSignalHomeEnvironment), &http.Client{Timeout: connectTimeout})
	fmt.Printf("%s%d\n", interactiveSignalHelperReturned, code)
	// 選択画面の読み取りの goroutine は os.Stdin の Read に残っている。ここで
	// 行を読むとそれと取り合うので、親がエコーを確かめて止めるまで待つだけにする。
	time.Sleep(time.Minute)
	os.Exit(code)
}

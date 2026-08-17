//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/platform/windowsregistry"
)

// recordingExecutable は、起こされた回数を数える本物の実行ファイルを作る。
//
// **偽の launcher では数えられない。** 確かめたいのは、記録された絶対パスが
// shell を介さずにそのまま実行されることなので、実行される側が本物でなければ
// 「何が起こされたか」を見たことにならない。
func recordingExecutable(t *testing.T) (path, ledger string) {
	t.Helper()
	directory := t.TempDir()
	ledger = filepath.Join(directory, "activations")
	source := filepath.Join(directory, "fake-desktop.go")
	program := `package main

import "os"

func main() {
	file, err := os.OpenFile(os.Getenv("SSHC_TEST_ACTIVATIONS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil { panic(err) }
	defer file.Close()
	if _, err := file.WriteString("started\n"); err != nil { panic(err) }
}
`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	// 名前は sshc.exe でなければならない。registry adapter がそこを見る。
	path = filepath.Join(directory, "sshc.exe")
	build := exec.Command("go", "build", "-o", path, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the fake desktop: %v\n%s", err, output)
	}
	t.Setenv("SSHC_TEST_ACTIVATIONS", ledger)
	return path, ledger
}

func activations(t *testing.T, ledger string) int {
	t.Helper()
	contents, err := os.ReadFile(ledger)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(contents), "started\n")
}

// registeredLauncher は、記録を読む側だけを差し替えた本物の launcher を返す。
//
// **実行そのものは production の経路を通る。** 差し替えるのを読み取りに留めて
// あるのは、本物のレジストリへ書くと、テストを走らせた人の機械にそのアプリの
// 起動登録が残るからである。書き込みと消去は windowsregistry 側が、自分の
// ためのテスト用の枝で確かめている。
func registeredLauncher(path string) desktopLauncher {
	return windowsDesktop{read: func() (string, error) { return path, nil }}
}

// **記録された絶対パスを、そのまま起こす。** shell も PATH も間に入らない。
func TestTheRegisteredExecutableIsStartedDirectly(t *testing.T) {
	path, ledger := recordingExecutable(t)
	launcher := registeredLauncher(path)

	if available, err := launcher.Available(); err != nil || !available {
		t.Fatalf("available = %v, %v", available, err)
	}
	if err := launcher.Launch(context.Background()); err != nil {
		t.Fatalf("launch: %v", err)
	}

	waitForActivations(t, ledger, 1)
}

// **裸の `sshc` も、施錠された窓を前へ出すのも、同じ道を一度だけ通る。**
// 二度起これば、二つ目の実体が上がって単一インスタンスの錠に弾かれる。
func TestBareActivationStartsTheDesktopExactlyOnce(t *testing.T) {
	path, ledger := recordingExecutable(t)
	launcher := registeredLauncher(path)

	code := runDesktop(context.Background(), t.TempDir(), &http.Client{}, launcher, &bytes.Buffer{})

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	waitForActivations(t, ledger, 1)
}

// **端末が持っている engine を、画面付きで起こし直さない。** そちらは誰かが
// 意図して走らせているもので、後から上げた外殻は lock を取れず死ぬ。
func TestALiveHeadlessOwnerIsNotDisplacedByTheRegisteredExecutable(t *testing.T) {
	path, ledger := recordingExecutable(t)
	stateDir := liveEngine(t, handoff.OwnerHeadless)
	launcher := registeredLauncher(path)
	var stderr bytes.Buffer

	code := runDesktop(context.Background(), stateDir, &http.Client{}, launcher, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if got := activations(t, ledger); got != 0 {
		t.Errorf("the registered executable ran %d times against a headless owner", got)
	}
	if !strings.Contains(stderr.String(), "headless") {
		t.Errorf("stderr = %q, want the headless owner named", stderr.String())
	}
}

// 登録が無ければ、起こす代わりに直し方を出す。**推測して何かを起こさない。**
func TestAnUnregisteredDesktopIsReportedRatherThanGuessed(t *testing.T) {
	launcher := windowsDesktop{read: func() (string, error) {
		return "", windowsregistry.ErrNotRegistered
	}}
	var stderr bytes.Buffer

	code := runDesktop(context.Background(), t.TempDir(), &http.Client{}, launcher, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "registered") {
		t.Errorf("stderr = %q, want the missing registration named", stderr.String())
	}
}

// waitForActivations は、起こされた回数がちょうど want になるのを待つ。
//
// **確かめているのは回数であって、速さではない。** 期限を短く取ると、他の
// パッケージと並んで走る実機で、正しく一度だけ起こしているものが落ちる——
// 新しく焼いた署名の無い実行ファイルは、初回の起動が数秒かかることがある。
// 一度も起こさない実装は何秒待っても 0 のままなので、余裕を取ってもこの主張は
// 立つ。
func waitForActivations(t *testing.T, ledger string, want int) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if activations(t, ledger) >= want {
			// 数え過ぎていないことも見る。**二度目は一度目より遅れて来る**ので、
			// 落ち着くだけの間を置いてから確かめる。
			time.Sleep(time.Second)
			if got := activations(t, ledger); got != want {
				t.Fatalf("the desktop was started %d times, want %d", got, want)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the desktop was started %d times, want %d", activations(t, ledger), want)
}

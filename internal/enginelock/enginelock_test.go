package enginelock

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	helperEnvironmentPath = "SSHC_ENGINELOCK_HELPER_PATH"
	helperEnvironmentMode = "SSHC_ENGINELOCK_HELPER_MODE"

	helperModeTry  = "try"
	helperModeHold = "hold"

	helperExitAcquired = 0
	helperExitFailed   = 1
	helperExitRunning  = 3
)

// TestMain はコンパイル済みテストバイナリを別の engine プロセスとして動作させる。
//
// 別プロセスから検査する必要がある。同じプロセスで 2 回目の Acquire を呼ぶだけでは、
// プロセスローカルなロックでも成功してしまう。
func TestMain(m *testing.M) {
	if path := os.Getenv(helperEnvironmentPath); path != "" {
		os.Exit(runLockHelper(path, os.Getenv(helperEnvironmentMode)))
	}
	os.Exit(m.Run())
}

func runLockHelper(path, mode string) int {
	report := func(line string) { _, _ = os.Stdout.WriteString(line + "\n") }
	release, err := Acquire(path)
	switch {
	case errors.Is(err, ErrRunning):
		report("busy")
		return helperExitRunning
	case err != nil:
		report("error: " + err.Error())
		return helperExitFailed
	}
	report("acquired")
	if mode == helperModeHold {
		// 親が stdin を閉じることが解放の合図である。sleep は関与しない。
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	if releaseErr := release(); releaseErr != nil {
		report("error: " + releaseErr.Error())
		return helperExitFailed
	}
	if mode == helperModeHold {
		report("released")
	}
	return helperExitAcquired
}

func helperCommand(t *testing.T, path, mode string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable)
	command.Env = append(os.Environ(), helperEnvironmentPath+"="+path, helperEnvironmentMode+"="+mode)
	return command
}

// lockInSeparateProcess は補助プロセスの出力 1 行と終了コードを返す。補助プロセスは
// この呼び出しより長く残らない。
func lockInSeparateProcess(t *testing.T, path string) (string, int) {
	t.Helper()
	command := helperCommand(t, path, helperModeTry)
	output, err := command.Output()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run separate engine process: %v", err)
	}
	return strings.TrimSpace(string(output)), command.ProcessState.ExitCode()
}

// heldLock は、このプロセスが stdin を閉じるか kill するまでロックを所有する別プロセスである。
type heldLock struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	output  *bufio.Reader
	stopped bool
}

func startHeldLock(t *testing.T, path string) *heldLock {
	t.Helper()
	command := helperCommand(t, path, helperModeHold)
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	held := &heldLock{command: command, stdin: stdin, output: bufio.NewReader(stdout)}
	t.Cleanup(held.stop)
	if line := held.line(t); line != "acquired" {
		t.Fatalf("holding process first line = %q, want acquired", line)
	}
	return held
}

func (h *heldLock) line(t *testing.T) string {
	t.Helper()
	line, err := h.output.ReadString('\n')
	if err != nil {
		t.Fatalf("read holding process output: %v", err)
	}
	return strings.TrimSpace(line)
}

func (h *heldLock) release(t *testing.T) {
	t.Helper()
	if err := h.stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if line := h.line(t); line != "released" {
		t.Fatalf("holding process release line = %q, want released", line)
	}
	if err := h.command.Wait(); err != nil {
		t.Fatalf("holding process exit = %v", err)
	}
	h.stopped = true
}

// kill は通常の解放処理を行わず所有プロセスを終了する。プロセスの回収完了を同期条件とし、
// 固定時間の sleep は使用しない。
func (h *heldLock) kill(t *testing.T) {
	t.Helper()
	if err := h.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = h.command.Wait()
	h.stopped = true
}

func (h *heldLock) stop() {
	if h.stopped {
		return
	}
	_ = h.stdin.Close()
	_ = h.command.Process.Kill()
	_ = h.command.Wait()
	h.stopped = true
}

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state", "engine.lock")
}

// エンジンが 2 台になる道を、ここで塞いでいる。1 台目が生きているあいだ、
// 別プロセスは必ず ErrRunning を受け取らなければならない。
func TestAcquireRefusesASecondProcessWhileTheLockIsHeld(t *testing.T) {
	path := lockPath(t)
	held := startHeldLock(t, path)

	if line, code := lockInSeparateProcess(t, path); line != "busy" || code != helperExitRunning {
		t.Fatalf("second engine process = %q, exit %d; want busy, exit %d", line, code, helperExitRunning)
	}

	held.release(t)

	if line, code := lockInSeparateProcess(t, path); line != "acquired" || code != helperExitAcquired {
		t.Fatalf("engine process after release = %q, exit %d; want acquired, exit 0", line, code)
	}
}

// プロセスが死ねば必ず外れる。O_EXCL で作ったファイルは、強制終了された
// 起動が置いていったものと、いま握られているものを区別できない。
func TestAcquireSucceedsAfterTheOwningProcessIsKilled(t *testing.T) {
	path := lockPath(t)
	startHeldLock(t, path).kill(t)

	if line, code := lockInSeparateProcess(t, path); line != "acquired" || code != helperExitAcquired {
		t.Fatalf("engine process after abnormal termination = %q, exit %d; want acquired, exit 0", line, code)
	}
}

func TestAcquireRefusesASecondEngineInTheSameProcess(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("the first engine could not take the lock: %v", err)
	}

	if _, err := Acquire(path); !errors.Is(err, ErrRunning) {
		t.Fatalf("the second engine got %v, want ErrRunning", err)
	}

	if err := release(); err != nil {
		t.Fatalf("release = %v", err)
	}
	next, err := Acquire(path)
	if err != nil {
		t.Fatalf("the lock stayed held after it was released: %v", err)
	}
	if err := next(); err != nil {
		t.Fatalf("release after reacquire = %v", err)
	}
}

// Release は Task 5 の終了経路から呼ばれる。そこでは同じ release が失敗経路と
// 正常経路の両方に現れうるので、二度呼ばれても同じ結果を返さなければならない。
func TestReleaseIsIdempotentAndSafeToCallConcurrently(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}

	results := make([]error, 8)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index] = release()
		}(index)
	}
	group.Wait()
	for index, releaseErr := range results {
		if releaseErr != nil {
			t.Fatalf("concurrent release %d = %v", index, releaseErr)
		}
	}
	if err := release(); err != nil {
		t.Fatalf("release after the concurrent calls = %v", err)
	}

	next, err := Acquire(path)
	if err != nil {
		t.Fatalf("the lock stayed held after repeated release: %v", err)
	}
	if err := next(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireCreatesTheMissingStateDirectory(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire with a missing parent = %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Fatal(err)
		}
	}()
	if info, statErr := os.Lstat(path); statErr != nil || !info.Mode().IsRegular() {
		t.Fatalf("lock file = %v, %v; want a regular file", info, statErr)
	}
}

// 取得に失敗した呼び出しは release を返さない。返していれば、呼び出し側は
// 握っていないロックを解放したつもりになる。
func TestFailedAcquireReturnsNoRelease(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()

	second, err := Acquire(path)
	if !errors.Is(err, ErrRunning) {
		t.Fatalf("second Acquire = %v, want ErrRunning", err)
	}
	if second != nil {
		t.Fatal("a refused Acquire returned a release function")
	}
}

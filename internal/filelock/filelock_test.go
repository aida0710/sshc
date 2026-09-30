package filelock

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
	"time"
)

const (
	helperEnvironmentPath = "SSHC_FILELOCK_HELPER_PATH"
	helperEnvironmentMode = "SSHC_FILELOCK_HELPER_MODE"

	helperModeTry  = "try"
	helperModeHold = "hold"

	helperExitAcquired = 0
	helperExitFailed   = 1
	helperExitHeld     = 3
)

// TestMain はコンパイル済みテストバイナリを、ロックを取る別プロセスとして動作させる。
//
// 別プロセスから検査する必要がある。同じプロセスで 2 回目の TryAcquire を呼ぶだけでは、
// プロセスローカルなロックでも成功してしまう。
func TestMain(m *testing.M) {
	if path := os.Getenv(helperEnvironmentPath); path != "" {
		os.Exit(runLockHelper(path, os.Getenv(helperEnvironmentMode)))
	}
	os.Exit(m.Run())
}

func runLockHelper(path, mode string) int {
	report := func(line string) { _, _ = os.Stdout.WriteString(line + "\n") }
	release, err := TryAcquire(path)
	switch {
	case errors.Is(err, ErrHeld):
		report("busy")
		return helperExitHeld
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
		t.Fatalf("run separate lock process: %v", err)
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

// 1 つ目のプロセスが握っているあいだ、別プロセスは必ず ErrHeld を受け取らなければ
// ならない。engine が 2 台になる道は、これで塞いでいる。
func TestTryAcquireRefusesASecondProcessWhileTheLockIsHeld(t *testing.T) {
	path := lockPath(t)
	held := startHeldLock(t, path)

	if line, code := lockInSeparateProcess(t, path); line != "busy" || code != helperExitHeld {
		t.Fatalf("second process = %q, exit %d; want busy, exit %d", line, code, helperExitHeld)
	}

	held.release(t)

	if line, code := lockInSeparateProcess(t, path); line != "acquired" || code != helperExitAcquired {
		t.Fatalf("process after release = %q, exit %d; want acquired, exit 0", line, code)
	}
}

// プロセスが死ねば必ず外れる。O_EXCL で作ったファイルは、強制終了された
// 起動が置いていったものと、いま握られているものを区別できない。
func TestTryAcquireSucceedsAfterTheOwningProcessIsKilled(t *testing.T) {
	path := lockPath(t)
	startHeldLock(t, path).kill(t)

	if line, code := lockInSeparateProcess(t, path); line != "acquired" || code != helperExitAcquired {
		t.Fatalf("process after abnormal termination = %q, exit %d; want acquired, exit 0", line, code)
	}
}

func TestTryAcquireRefusesASecondHolderInTheSameProcess(t *testing.T) {
	path := lockPath(t)
	release, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("the first holder could not take the lock: %v", err)
	}

	if _, err := TryAcquire(path); !errors.Is(err, ErrHeld) {
		t.Fatalf("the second holder got %v, want ErrHeld", err)
	}

	if err := release(); err != nil {
		t.Fatalf("release = %v", err)
	}
	next, err := TryAcquire(path)
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
	release, err := TryAcquire(path)
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

	next, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("the lock stayed held after repeated release: %v", err)
	}
	if err := next(); err != nil {
		t.Fatal(err)
	}
}

func TestTryAcquireCreatesTheMissingStateDirectory(t *testing.T) {
	path := lockPath(t)
	release, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire with a missing parent = %v", err)
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
func TestFailedTryAcquireReturnsNoRelease(t *testing.T) {
	path := lockPath(t)
	release, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()

	second, err := TryAcquire(path)
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("second TryAcquire = %v, want ErrHeld", err)
	}
	if second != nil {
		t.Fatal("a refused TryAcquire returned a release function")
	}
}

// AcquireWithin は、握っている相手が wait のうちに外せば、そのあとで取る。
// 外すまでは返らない。
func TestAcquireWithinTakesTheLockOnceTheHolderReleasesIt(t *testing.T) {
	path := lockPath(t)
	holder, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}

	type acquisition struct {
		release func() error
		err     error
	}
	acquired := make(chan acquisition, 1)
	go func() {
		release, err := AcquireWithin(path, time.Minute)
		acquired <- acquisition{release, err}
	}()

	select {
	case got := <-acquired:
		if got.release != nil {
			_ = got.release()
		}
		t.Fatalf("AcquireWithin returned while the lock was held: %v", got.err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := holder(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-acquired:
		if got.err != nil {
			t.Fatalf("AcquireWithin after release = %v", got.err)
		}
		if err := got.release(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("AcquireWithin did not take the lock after the holder released it")
	}
}

// 相手が wait を過ぎても握っていれば、ErrHeld で諦め、release は返さない。
func TestAcquireWithinReturnsErrHeldWhenTheWaitRunsOut(t *testing.T) {
	path := lockPath(t)
	holder, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder() }()

	release, err := AcquireWithin(path, 30*time.Millisecond)
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("AcquireWithin past the wait = %v, want ErrHeld", err)
	}
	if release != nil {
		t.Fatal("a refused AcquireWithin returned a release function")
	}
}

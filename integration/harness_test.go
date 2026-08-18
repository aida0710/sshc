// Package integration は、本物の sshc プロセスに対して契約を確かめる。
//
// **helper process ではなく、ビルドした実体を起こす。** ここで確かめるのは
// 所有権であり、それはプロセスと OS のロックとパイプの上にしかない。同じ
// プロセスの中で関数を呼び合っても、二台目が一台目のロックに弾かれることも、
// パイプが閉じて子が畳まれることも起きない。
//
// **HOME はテストごとに隔離する。** ここが起こすのは engine であり、engine は
// 利用者の ~/.ssh に触る。本物の HOME を見せれば、テストが開発者の handoff を
// 書き換え、走っているアプリのロックを奪う。
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// binaryPath は、このパッケージが起こす sshc の実体である。TestMain が一度だけ
// ビルドする。
var binaryPath string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "sshc-integration-")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()

	binaryPath = filepath.Join(directory, "sshc")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/sshc")
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		panic(string(output))
	}
	os.Exit(m.Run())
}

// lockedBuffer は、走っている子の出力を、テストが読みながら集める。
//
// bytes.Buffer をそのまま渡すと、書くのは exec の goroutine で読むのはテストに
// なる。`go test -race` はそれを見つけるし、見つからなくても壊れている。
type lockedBuffer struct {
	mutex   sync.Mutex
	content bytes.Buffer
}

func (buffer *lockedBuffer) Write(chunk []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.content.Write(chunk)
}

func (buffer *lockedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.content.String()
}

// testProcess は、起こした sshc ひとつである。
type testProcess struct {
	Command *exec.Cmd
	Stdout  *lockedBuffer
	Stderr  *lockedBuffer
	// Ownership は、engine の所有権チャンネルである。desktop の owner だけが
	// 持ち、閉じることが「もう要らない」という意味になる。
	Ownership *os.File
	exited    chan error
}

// isolatedHome は、このテストひとりのための家を作る。
func isolatedHome(t *testing.T) string {
	t.Helper()
	// macOS の TempDir は /var 経由の symlink を含みうる。engine は state
	// ディレクトリを実体として扱うので、先に解いておく。
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

// osPath と osSystemRoot は、隔離した環境でも渡さざるを得ないものである。
// 前者は engine が報告対象のプログラムを見つけるため、後者は Windows の
// プロセスが DLL を読むために要る。
func osPath() string       { return os.Getenv("PATH") }
func osSystemRoot() string { return os.Getenv("SystemRoot") }

// start は、隔離された家の中で sshc をひとつ起こす。
func start(t *testing.T, home string, args ...string) *testProcess {
	t.Helper()
	command := exec.Command(binaryPath, args...)
	// **本物の環境をそのまま渡さない。** PATH と、Go が家を探すのに使う名前
	// だけを与える。ここに残った HOME や USERPROFILE がひとつでも通れば、
	// engine は開発者の ~/.ssh を開く。
	command.Env = []string{
		"PATH=" + osPath(),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + home,
		"SystemRoot=" + osSystemRoot(),
	}
	return startPrepared(t, command)
}

// startPrepared は、組み立て済みの command を起こし、出力と終了を見張る。
func startPrepared(t *testing.T, command *exec.Cmd) *testProcess {
	t.Helper()
	process := &testProcess{
		Command: command,
		Stdout:  &lockedBuffer{},
		Stderr:  &lockedBuffer{},
		exited:  make(chan error, 1),
	}
	command.Stdout = process.Stdout
	command.Stderr = process.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { process.exited <- command.Wait() }()
	t.Cleanup(func() { process.kill() })
	return process
}

// startOwned は、Electron と同じ形で engine を起こす。所有権は書ける側の
// パイプであり、それを閉じることが engine への「終わってよい」である。
func startOwned(t *testing.T, home string) *testProcess {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binaryPath, "engine")
	command.Env = []string{
		"PATH=" + osPath(),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + home,
		"SystemRoot=" + osSystemRoot(),
	}
	command.Stdin = reader
	process := &testProcess{
		Command:   command,
		Stdout:    &lockedBuffer{},
		Stderr:    &lockedBuffer{},
		Ownership: writer,
		exited:    make(chan error, 1),
	}
	command.Stdout = process.Stdout
	command.Stderr = process.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	// 親の側は子へ渡した端を手放す。持ったままだと、writer を閉じても子には
	// EOF が届かない。
	_ = reader.Close()
	go func() { process.exited <- command.Wait() }()
	t.Cleanup(func() {
		_ = writer.Close()
		process.kill()
	})
	return process
}

// wait は、終了コードを返す。時間内に終わらなければテストを落とす。
func (process *testProcess) wait(t *testing.T, within time.Duration) int {
	t.Helper()
	select {
	case err := <-process.exited:
		if err == nil {
			return 0
		}
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("wait: %v\n%s", err, process.Stderr.String())
		}
		return exit.ExitCode()
	case <-time.After(within):
		t.Fatalf("the process was still running after %s\nstdout: %s\nstderr: %s",
			within, process.Stdout.String(), process.Stderr.String())
		return -1
	}
}

// running は、いまも走っているかを答える。
func (process *testProcess) running() bool {
	select {
	case err := <-process.exited:
		process.exited <- err
		return false
	default:
		return true
	}
}

func (process *testProcess) kill() {
	if process.Command.Process != nil {
		_ = process.Command.Process.Kill()
	}
}

// stateDir は、この家の engine が状態を置く場所である。
func stateDir(home string) string {
	return filepath.Join(home, ".ssh", "sshc")
}

// handoffPath は、この家の handoff 文書の場所である。名前は handoff.FileName が
// 決めるので、ここで綴り直さない。
func handoffPath(home string) string {
	return filepath.Join(stateDir(home), handoff.FileName)
}

func lockPath(home string) string {
	return filepath.Join(stateDir(home), "engine.lock")
}

// waitForFile は、engine が何かを置くまで待つ。
func waitForFile(t *testing.T, path string, within time.Duration, process *testProcess) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if process != nil && !process.running() {
			t.Fatalf("the engine exited before writing %s\nstdout: %s\nstderr: %s",
				path, process.Stdout.String(), process.Stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

// takeOverAsHeadless は、次の owner が席に着いて **自分の名前で handoff を
// 書き直す**まで待つ。
//
// **handoff が在ることを、席を取れた証拠にしない。** 殺された engine の
// handoff はそのまま残っているので、それを見ると、ロックに弾かれて今まさに
// 終わろうとしているプロセスを成功として返す。名乗っている pid が起こした
// ものと一致することだけが証拠になる。
//
// **一度目で立てられることも前提にしない。** 殺されたプロセスのロックが OS
// から外れるのは、その終了を親が回収した瞬間とは限らない——Windows の
// LockFileEx はプロセスオブジェクトの破棄に伴って外れるので、混んだ機械では
// 少し遅れる。製品はその間、正しく「既に走っている」と断る。確かめたいのは
// 「席は必ず空く」ことであって、「一度目で空いている」ことではない。
func takeOverAsHeadless(t *testing.T, home string) *testProcess {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last *testProcess
	for time.Now().Before(deadline) {
		next := start(t, home, "headless")
		settled := time.Now().Add(15 * time.Second)
		for time.Now().Before(settled) {
			if !next.running() {
				break
			}
			if document, err := handoff.Read(stateDir(home)); err == nil &&
				document.PID == next.Command.Process.Pid {
				return next
			}
			time.Sleep(20 * time.Millisecond)
		}
		last = next
		next.kill()
		time.Sleep(100 * time.Millisecond)
	}
	reason := ""
	if last != nil {
		reason = last.Stderr.String()
	}
	t.Fatalf("no later owner named itself in the handoff within 60s\n%s", reason)
	return nil
}

// waitFor は、条件が満たされるまで待つ。
func waitFor(t *testing.T, within time.Duration, describe string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not happen within %s", describe, within)
}

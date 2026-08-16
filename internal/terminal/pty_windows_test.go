//go:build windows

package terminal_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"sshc/internal/terminal"
)

// このテストのいくつかは、自分自身を子として起こす。
//
// **PowerShell の REPL を相手にしない。** 大きさを確かめるには resize のあとに
// もう一度問い直す子が要る。それを PSReadLine の VT 混じりのエコーから読み取る
// のは、この表明を最も壊れやすい形で書くことになる。
const (
	sizeHelperEnvironment       = "SSHC_TERMINAL_SIZE_HELPER"
	descendantHelperEnvironment = "SSHC_TERMINAL_DESCENDANT_HELPER"
)

func TestMain(m *testing.M) {
	if os.Getenv(sizeHelperEnvironment) != "" {
		os.Exit(runSizeHelper())
	}
	if os.Getenv(descendantHelperEnvironment) != "" {
		os.Exit(runDescendantHelper())
	}
	os.Exit(m.Run())
}

// runSizeHelper は、stdin の 1 行ごとに、いまのコンソールの大きさを出す。
func runSizeHelper() int {
	report := func() {
		var info windows.ConsoleScreenBufferInfo
		handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
		if err != nil {
			fmt.Printf("error %v\n", err)
			return
		}
		if err := windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
			fmt.Printf("error %v\n", err)
			return
		}
		fmt.Printf("size %dx%d\n", info.Window.Right-info.Window.Left+1, info.Window.Bottom-info.Window.Top+1)
	}
	report()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		report()
	}
	return 0
}

// runDescendantHelper は、孫をひとつ起こしてその pid を出し、そのまま待つ。
func runDescendantHelper() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Printf("error %v\n", err)
		return 1
	}
	// 孫は自分のコンソールを持つ。**擬似コンソールを閉じても死なない**——
	// これを取り除けるのはジョブだけである。
	command := fmt.Sprintf(`"%s" -test.run=TestNeverRuns`, executable)
	commandLine, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return 1
	}
	var startup windows.StartupInfo
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var information windows.ProcessInformation
	if err := windows.CreateProcess(nil, commandLine, nil, nil, false,
		windows.CREATE_NEW_CONSOLE, nil, nil, &startup, &information); err != nil {
		fmt.Printf("error %v\n", err)
		return 1
	}
	windows.CloseHandle(information.Thread)
	windows.CloseHandle(information.Process)
	fmt.Printf("descendant %d\n", information.ProcessId)
	select {}
}

func powershell(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Skip("SystemRoot is not set, so PowerShell's absolute path is unknown")
	}
	path := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("PowerShell is not at %s: %v", path, err)
	}
	return path
}

func selfCommand(t *testing.T, environment string) terminal.Command {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return terminal.Command{
		Path:      executable,
		Arguments: []string{"-test.run=TestNeverRuns"},
		Env:       append(os.Environ(), environment+"=1"),
	}
}

func readUntil(t *testing.T, process terminal.Process, want string, limit time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(limit)
	var collected strings.Builder
	buffer := make([]byte, 4096)
	for time.Now().Before(deadline) {
		read, err := process.Read(buffer)
		if read > 0 {
			collected.Write(buffer[:read])
			if strings.Contains(collected.String(), want) {
				return collected.String()
			}
		}
		if err != nil {
			break
		}
	}
	t.Fatalf("never saw %q; console output was:\n%s", want, collected.String())
	return ""
}

// **子が自分で終われば、読み取りは EOF に届かなければならない。**
//
// 擬似コンソール側のパイプ端を手放し忘れると、出力パイプに書き手が残り続け、
// Read は永久に返らない。そうなると pump は done を閉じず、Registry.Wait は
// 戻らず、engine lock はそのマシンが再起動するまで握られたままになる。
func TestWindowsConsoleReachesEOFWhenTheChildExitsOnItsOwn(t *testing.T) {
	shell := powershell(t)
	process, err := terminal.NewStarter().Start(context.Background(), terminal.Command{
		Path:      shell,
		Arguments: []string{"-NoProfile", "-Command", "exit 7"},
	}, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })

	buffer := make([]byte, 4096)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("the console output never reached EOF after the child exited")
		}
		if _, err := process.Read(buffer); err != nil {
			break
		}
	}
	info := process.Wait()
	if info.Code != 7 || info.Signal != "" {
		t.Fatalf("exit = %#v, want code 7 and no signal", info)
	}
}

func TestWindowsConsoleCarriesAMarkerBackFromPowerShell(t *testing.T) {
	shell := powershell(t)
	process, err := terminal.NewStarter().Start(context.Background(), terminal.Command{
		Path:      shell,
		Arguments: []string{"-NoProfile", "-Command", "Write-Output 'sshc-marker-9f2c'"},
	}, terminal.Size{Cols: 120, Rows: 30})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "sshc-marker-9f2c", 30*time.Second)
}

func TestWindowsConsoleReportsItsSizeAndTheSizeAfterAResize(t *testing.T) {
	process, err := terminal.NewStarter().Start(context.Background(),
		selfCommand(t, sizeHelperEnvironment), terminal.Size{Cols: 100, Rows: 40})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })

	readUntil(t, process, "size 100x40", 30*time.Second)

	if err := process.Resize(terminal.Size{Cols: 132, Rows: 50}); err != nil {
		t.Fatalf("Resize = %v", err)
	}
	if _, err := process.Write([]byte("\r\n")); err != nil {
		t.Fatalf("Write = %v", err)
	}
	readUntil(t, process, "size 132x50", 30*time.Second)
}

// 孫は自分のコンソールを持つので、擬似コンソールを閉じても生き残る。
// **取り除けるのはジョブだけである。**
func TestWindowsClosingRemovesADescendantWithItsOwnConsole(t *testing.T) {
	assertDescendantIsRemoved(t, func(process terminal.Process) {
		if err := process.Hangup(); err != nil {
			t.Fatalf("Hangup = %v", err)
		}
		if err := process.Close(); err != nil {
			t.Fatalf("Close = %v", err)
		}
	})
}

func TestWindowsForceCloseRemovesADescendantWithItsOwnConsole(t *testing.T) {
	assertDescendantIsRemoved(t, func(process terminal.Process) {
		forcer, ok := process.(interface{ ForceClose() error })
		if !ok {
			t.Fatal("the console process has no force hook")
		}
		if err := forcer.ForceClose(); err != nil {
			t.Fatalf("ForceClose = %v", err)
		}
	})
}

func assertDescendantIsRemoved(t *testing.T, teardown func(terminal.Process)) {
	t.Helper()
	process, err := terminal.NewStarter().Start(context.Background(),
		selfCommand(t, descendantHelperEnvironment), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })

	output := readUntil(t, process, "descendant ", 30*time.Second)
	index := strings.Index(output, "descendant ")
	fields := strings.Fields(output[index:])
	if len(fields) < 2 {
		t.Fatalf("could not read the descendant's pid from %q", output)
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 32)
	if err != nil {
		t.Fatalf("descendant pid = %q: %v", fields[1], err)
	}

	// **まだ生きているうちにハンドルを取る。** そうすることで pid の使い回しに
	// 引っかからない。
	handle, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess(%d) = %v", pid, err)
	}
	defer windows.CloseHandle(handle)

	teardown(process)

	event, err := windows.WaitForSingleObject(handle, 30_000)
	if err != nil {
		t.Fatalf("WaitForSingleObject = %v", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		t.Fatalf("the descendant was still alive 30s after teardown (wait = %#x)", event)
	}
}

// Registry.discard はこの順で呼ぶ。**それでも強制した事実が報告されなければ
// ならない。**
func TestWindowsForceThenCloseThenWaitStillReportsTheForcedExit(t *testing.T) {
	process, err := terminal.NewStarter().Start(context.Background(),
		selfCommand(t, sizeHelperEnvironment), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	forcer, ok := process.(interface{ ForceClose() error })
	if !ok {
		t.Fatal("the console process has no force hook")
	}
	if err := forcer.ForceClose(); err != nil {
		t.Fatalf("ForceClose = %v", err)
	}
	if err := process.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	info := process.Wait()
	if info.Signal != "killed" || info.Code != -1 {
		t.Fatalf("forced exit = %#v, want code -1 and signal killed", info)
	}
}

// 大きさの正規化は Registry の仕事なので、**起動そのものは自分で拒む**。
func TestWindowsStartRefusesWhatItCannotRun(t *testing.T) {
	starter := terminal.NewStarter()
	shell := powershell(t)
	for name, test := range map[string]struct {
		command terminal.Command
		size    terminal.Size
	}{
		"invalid size":       {terminal.Command{Path: shell}, terminal.Size{Cols: 0, Rows: 0}},
		"relative path":      {terminal.Command{Path: "powershell.exe"}, terminal.Size{Cols: 80, Rows: 24}},
		"empty path":         {terminal.Command{}, terminal.Size{Cols: 80, Rows: 24}},
		"missing executable": {terminal.Command{Path: filepath.Join(t.TempDir(), "absent.exe")}, terminal.Size{Cols: 80, Rows: 24}},
	} {
		t.Run(name, func(t *testing.T) {
			process, err := starter.Start(context.Background(), test.command, test.size)
			if err == nil {
				_ = process.Close()
				t.Fatal("Start accepted a request it cannot run")
			}
		})
	}
}

// 取り消された確保は、何も残さない。
func TestWindowsStartRefusesACancelledCreation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shell := powershell(t)
	if _, err := terminal.NewStarter().Start(ctx, terminal.Command{Path: shell},
		terminal.Size{Cols: 80, Rows: 24}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start with a cancelled creation = %v, want context.Canceled", err)
	}
}

// **環境ブロックを途中で終わらせられる項目は受け取らない。** 一項目で子の
// 環境全体を差し替えられてしまう。
func TestWindowsStartRefusesAnUnusableEnvironmentEntry(t *testing.T) {
	shell := powershell(t)
	for name, environment := range map[string][]string{
		"embedded NUL":  {"GOOD=1", "BAD\x00=2"},
		"no assignment": {"GOOD=1", "JUSTANAME"},
	} {
		t.Run(name, func(t *testing.T) {
			process, err := terminal.NewStarter().Start(context.Background(),
				terminal.Command{Path: shell, Env: environment}, terminal.Size{Cols: 80, Rows: 24})
			if err == nil {
				_ = process.Close()
				t.Fatal("Start accepted an environment entry that can truncate the block")
			}
		})
	}
}

//go:build windows

package terminal_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"sshc/internal/terminal"
)

const (
	sizeHelperEnvironment       = "SSHC_TERMINAL_SIZE_HELPER"
	descendantHelperEnvironment = "SSHC_TERMINAL_DESCENDANT_HELPER"
	idleHelperEnvironment       = "SSHC_TERMINAL_IDLE_HELPER"
	exitHelperEnvironment       = "SSHC_TERMINAL_EXIT_HELPER"
)

func TestMain(m *testing.M) {
	if os.Getenv(sizeHelperEnvironment) != "" {
		os.Exit(runSizeHelper())
	}
	if os.Getenv(descendantHelperEnvironment) != "" {
		os.Exit(runDescendantHelper())
	}
	if os.Getenv(idleHelperEnvironment) != "" {
		os.Exit(runIdleHelper())
	}
	if os.Getenv(exitHelperEnvironment) != "" {
		os.Exit(7)
	}
	os.Exit(m.Run())
}

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

func runDescendantHelper() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Printf("error %v\n", err)
		return 1
	}
	grandchild := exec.Command(executable, "-test.run=TestNeverRuns")
	grandchild.Env = []string{idleHelperEnvironment + "=1", "SystemRoot=" + os.Getenv("SystemRoot")}
	grandchild.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := grandchild.Start(); err != nil {
		fmt.Printf("error %v\n", err)
		return 1
	}
	fmt.Printf("descendant %d\n", grandchild.Process.Pid)
	time.Sleep(10 * time.Minute)
	return 0
}

func runIdleHelper() int {
	time.Sleep(10 * time.Minute)
	return 0
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
	type outcome struct {
		text  string
		found bool
	}
	answers := make(chan outcome, 1)
	go func() {
		var collected strings.Builder
		buffer := make([]byte, 4096)
		for {
			read, err := process.Read(buffer)
			if read > 0 {
				collected.Write(buffer[:read])
				if strings.Contains(collected.String(), want) {
					answers <- outcome{collected.String(), true}
					return
				}
			}
			if err != nil {
				answers <- outcome{collected.String(), false}
				return
			}
		}
	}()
	select {
	case answer := <-answers:
		if !answer.found {
			t.Fatalf("the console ended before %q appeared; output was:\n%s", want, answer.text)
		}
		return answer.text
	case <-time.After(limit):
		t.Fatalf("never saw %q within %v", want, limit)
		return ""
	}
}

func TestWindowsConsoleReachesEOFWhenTheChildExitsOnItsOwn(t *testing.T) {
	process, err := terminal.NewStarter().Start(context.Background(),
		selfCommand(t, exitHelperEnvironment), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = process.Close() })

	ended := make(chan struct{}, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			if _, err := process.Read(buffer); err != nil {
				ended <- struct{}{}
				return
			}
		}
	}()
	select {
	case <-ended:
	case <-time.After(30 * time.Second):
		t.Fatal("the console output never reached EOF after the child exited")
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

func TestWindowsStartRefusesACancelledCreation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shell := powershell(t)
	if _, err := terminal.NewStarter().Start(ctx, terminal.Command{Path: shell},
		terminal.Size{Cols: 80, Rows: 24}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start with a cancelled creation = %v, want context.Canceled", err)
	}
}

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

func TestWindowsResizeAfterTheConsoleIsGoneDoesNotTouchIt(t *testing.T) {
	shell := powershell(t)
	process, err := terminal.NewStarter().Start(context.Background(), terminal.Command{
		Path:      shell,
		Arguments: []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 60"},
	}, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}

	if err := process.Hangup(); err != nil {
		t.Fatalf("Hangup = %v", err)
	}
	if err := process.Resize(terminal.Size{Cols: 100, Rows: 40}); err != nil {
		t.Fatalf("Resize after Hangup = %v, want it to be a quiet no-op", err)
	}

	if err := process.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := process.Resize(terminal.Size{Cols: 120, Rows: 50}); err != nil {
		t.Fatalf("Resize after Close = %v, want it to be a quiet no-op", err)
	}
	forcer, ok := process.(interface{ ForceClose() error })
	if !ok {
		t.Fatal("the console process has no force hook")
	}
	if err := forcer.ForceClose(); err != nil {
		t.Fatalf("ForceClose after Close = %v, want it to be a quiet no-op", err)
	}
	process.Wait()
}

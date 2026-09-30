//go:build !darwin && !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// launcherWriteDeadline は、xdg-open の代わりの sh が 1 行を書き終えるのを待つ上限。
// 起動の遅い CI でも十分な長さにする。
const launcherWriteDeadline = 5 * time.Second

// installFakeXDGOpen は、xdg-open の代わりに、report が標準出力へ書いた 1 行を
// ファイルへ残す sh を置き、そのファイルのパスを返す。
func installFakeXDGOpen(t *testing.T, report string) string {
	t.Helper()
	directory := t.TempDir()
	marker := filepath.Join(directory, "opened")
	launcher := "#!/bin/sh\n{ " + report + "; } > '" + marker + "'\n"
	if err := os.WriteFile(filepath.Join(directory, "xdg-open"), []byte(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("DISPLAY", ":0")
	return marker
}

// waitForLauncherReport は、偽の xdg-open が改行で終わる 1 行を書き終えるまで待ち、
// 改行を除いたその行を返す。
func waitForLauncherReport(t *testing.T, marker string) string {
	t.Helper()
	deadline := time.Now().Add(launcherWriteDeadline)
	for {
		written, err := os.ReadFile(marker)
		if line, complete := strings.CutSuffix(string(written), "\n"); err == nil && complete {
			return line
		}
		if time.Now().After(deadline) {
			t.Fatalf("launcher wrote %q (err %v) within %s, want one line", written, err, launcherWriteDeadline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOpenInBrowserLetsTheLauncherFinishAfterReturning(t *testing.T) {
	marker := installFakeXDGOpen(t, `printf '%s\n' "$1"`)
	const url = "http://127.0.0.1:1/#bootstrap=test"

	if !openInBrowser(url) {
		t.Fatal("openInBrowser = false, want true")
	}

	if opened := waitForLauncherReport(t, marker); opened != url {
		t.Fatalf("launcher opened %q, want %q", opened, url)
	}
}

// ターミナルが前面の process group へ送る SIGINT・SIGHUP を、開いたブラウザに
// 届かせない。
func TestOpenInBrowserStartsTheLauncherOutsideTheCallersProcessGroupAndSession(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("this system has no /proc/<pid>/stat to report the launcher's process group")
	}
	// /proc/<pid>/stat の 5 番目と 6 番目の欄が process group と session である。
	marker := installFakeXDGOpen(t, `read -r _ _ _ _ group session _ < /proc/$$/stat; printf '%s %s\n' "$group" "$session"`)

	if !openInBrowser("http://127.0.0.1:1/#bootstrap=test") {
		t.Fatal("openInBrowser = false, want true")
	}

	var launcherGroup, launcherSession int
	report := waitForLauncherReport(t, marker)
	if _, err := fmt.Sscan(report, &launcherGroup, &launcherSession); err != nil {
		t.Fatalf("launcher reported %q: %v", report, err)
	}
	callerSession, err := unix.Getsid(0)
	if err != nil {
		t.Fatal(err)
	}
	if launcherGroup == unix.Getpgrp() {
		t.Errorf("launcher process group = %d, the same as the caller's", launcherGroup)
	}
	if launcherSession == callerSession {
		t.Errorf("launcher session = %d, the same as the caller's", launcherSession)
	}
}

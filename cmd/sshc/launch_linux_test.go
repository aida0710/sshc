//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeDescriptor(t *testing.T, stateDir, executable string) {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := `{"executable":` + quoteJSON(executable) + `}`
	if err := os.WriteFile(filepath.Join(stateDir, desktopDescriptorName), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

func executableFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sshc.AppImage")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func withDisplay(value string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if name == "DISPLAY" && value != "" {
			return value, true
		}
		return "", false
	}
}

func TestLinuxDesktopIsUnavailableWithoutADisplay(t *testing.T) {
	stateDir := t.TempDir()
	writeDescriptor(t, stateDir, executableFile(t))
	launcher := linuxDesktop{stateDir: stateDir, lookup: withDisplay("")}

	available, err := launcher.Available()

	if available {
		t.Error("available = true without a display")
	}
	// 画面が無いのは壊れているのではない。直し方を出すと、直すもののない利用者に
	// 直せと言うことになる——そちらの正しい答えは sshc headless である。
	if err != nil {
		t.Errorf("err = %v, want nil; a missing display is not a broken installation", err)
	}
}

func TestLinuxDesktopReportsAMovedAppImage(t *testing.T) {
	stateDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "moved-away.AppImage")
	writeDescriptor(t, stateDir, missing)
	launcher := linuxDesktop{stateDir: stateDir, lookup: withDisplay(":0")}

	available, err := launcher.Available()

	if available {
		t.Error("available = true for a recorded path that is gone")
	}
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("err = %v, want the recorded absolute path named", err)
	}
}

func TestLinuxDesktopRefusesEverythingButAnAbsoluteExecutableFile(t *testing.T) {
	directory := t.TempDir()
	plain := filepath.Join(directory, "not-executable")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"empty":          "",
		"relative":       "sshc.AppImage",
		"dot relative":   "./sshc.AppImage",
		"directory":      directory,
		"not executable": plain,
	} {
		if err := validateDesktopExecutable(path); err == nil {
			t.Errorf("%s (%q) was accepted as a desktop executable", name, path)
		}
	}
	if err := validateDesktopExecutable(executableFile(t)); err != nil {
		t.Errorf("an absolute executable regular file was refused: %v", err)
	}
}

func TestLinuxDesktopStartsTheRecordedFileDirectly(t *testing.T) {
	stateDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	script := filepath.Join(t.TempDir(), "sshc.AppImage")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDescriptor(t, stateDir, script)
	launcher := linuxDesktop{stateDir: stateDir, lookup: withDisplay(":0")}

	if err := launcher.Launch(context.Background()); err != nil {
		t.Fatalf("launch: %v", err)
	}
	// Start は待たないので、印が付くまで少し待つ。待つのはテストであって、
	// 端末を返すまでの経路ではない。
	waitForFile(t, marker)
}

func TestLinuxDesktopWithoutADescriptorAsksForTheAppImage(t *testing.T) {
	launcher := linuxDesktop{stateDir: t.TempDir(), lookup: withDisplay(":0")}

	available, err := launcher.Available()

	if available {
		t.Error("available = true with no descriptor")
	}
	if err == nil || !strings.Contains(err.Error(), "AppImage") {
		t.Errorf("err = %v, want the AppImage named", err)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s was never created", path)
}

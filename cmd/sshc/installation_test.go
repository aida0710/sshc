package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellReceiptBindsTheExecutableDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.shのreceiptはWindowsでは更新対象にしない")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "sshc")
	contents := []byte("a published sshc binary")
	if err := os.WriteFile(executable, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	receipt := fmt.Sprintf(`{"schemaVersion":1,"manager":"install.sh","repository":"aida0710/sshc","version":"v0.13.6","sha256":"%s"}`,
		hex.EncodeToString(digest[:]))
	if err := os.WriteFile(filepath.Join(directory, receiptFileName), []byte(receipt), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := detectInstallation(executable)
	if err != nil || found.manager != managerShell {
		t.Fatalf("detect = %#v, %v", found, err)
	}
	if err := os.WriteFile(executable, []byte("manually replaced"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 一致しないreceiptは自動更新の根拠にしない。直し方も合わせて伝える。
	_, err = detectInstallation(executable)
	if err == nil || !strings.Contains(err.Error(), "does not match") ||
		!strings.Contains(err.Error(), "install sshc again with install.sh") {
		t.Fatalf("modified receipt error = %v", err)
	}
}

func TestShellReceiptDoesNotFollowASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available on Windows")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "sshc")
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	realReceipt := filepath.Join(directory, "elsewhere.json")
	if err := os.WriteFile(realReceipt, []byte(`{"schemaVersion":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realReceipt, filepath.Join(directory, receiptFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := detectInstallation(executable); err == nil || !strings.Contains(err.Error(), "regular install receipt") {
		t.Fatalf("symlink receipt error = %v", err)
	}
}

func TestHomebrewCandidateRequiresItsOwnExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew is not supported on Windows")
	}
	prefix := t.TempDir()
	kegBinary := filepath.Join(prefix, "Cellar", "sshc", "0.13.6", "bin", "sshc")
	if err := os.MkdirAll(filepath.Dir(kegBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kegBinary, []byte("brew sshc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(prefix, "bin", "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := detectInstallation(kegBinary)
	if err != nil || found.manager != managerHomebrew {
		t.Fatalf("detect = %#v, %v", found, err)
	}
	foundInfo, err := os.Stat(found.brew)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(brew)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(foundInfo, wantInfo) {
		t.Fatalf("detected brew %q is not the fixture %q", found.brew, brew)
	}
}

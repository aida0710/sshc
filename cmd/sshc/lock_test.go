package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/app"
)

func mustStateDir(t *testing.T, home string) string {
	t.Helper()
	stateDir, err := app.StateDir(home)
	if err != nil {
		t.Fatalf("StateDir(%q) = %v", home, err)
	}
	return stateDir
}

// GNU stow などで ~/.ssh が symlink になっていても、解決した state directory の
// engine.lock は取れる。symlink を拒む no-follow の歩き方で未解決のパスを開くと、
// engine が起動できなかった。
func TestLockEngineStartWorksWhenTheSSHDirectoryIsASymlink(t *testing.T) {
	home := t.TempDir()
	linkedSSH := filepath.Join(home, "dotfiles", "ssh")
	if err := os.MkdirAll(linkedSSH, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedSSH, filepath.Join(home, ".ssh")); err != nil {
		t.Skipf("this platform cannot create the symlink: %v", err)
	}

	release, err := lockEngineStart(mustStateDir(t, home))
	if err != nil {
		t.Fatalf("take the engine lock under a symlinked ~/.ssh = %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release = %v", err)
	}
}

// CLI が状態ディレクトリ内の同じ engine.lock を使用することを検証する。
func TestLockEngineStartRefusesASecondEngine(t *testing.T) {
	stateDir := t.TempDir()

	release, err := lockEngineStart(stateDir)
	if err != nil {
		t.Fatalf("the first engine could not take the lock: %v", err)
	}

	if _, err := lockEngineStart(stateDir); !errors.Is(err, errEngineRunning) {
		t.Fatalf("the second engine got %v, want errEngineRunning", err)
	}

	if _, statErr := os.Lstat(filepath.Join(stateDir, "engine.lock")); statErr != nil {
		t.Fatalf("engine.lock is not in the state directory: %v", statErr)
	}

	// 解放後に別の engine が lock を取得できることを検証する。
	if err := release(); err != nil {
		t.Fatalf("release = %v", err)
	}
	next, err := lockEngineStart(stateDir)
	if err != nil {
		t.Fatalf("the lock stayed held after it was released: %v", err)
	}
	if err := next(); err != nil {
		t.Fatal(err)
	}
}

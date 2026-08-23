package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

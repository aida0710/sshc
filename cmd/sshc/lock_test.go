package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// **エンジンが 2 台になる道を、ここで塞いでいる。** 裸の `sshc` はサブコマンドの
// どれにも当たらないので、これが無ければそのまま 2 台目が上がる。ロックの実体と
// 実プロセス試験は internal/enginelock にあり、ここが確かめるのは、この
// コマンドが同じ状態ディレクトリの同じ engine.lock を名指すことである。
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

	// **プロセスが死ねば必ず外れる。** 手放したあとは次の 1 台が上がれる
	// ——外れないロックは、誰もエンジンを起こせない状態を永久に残す。
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

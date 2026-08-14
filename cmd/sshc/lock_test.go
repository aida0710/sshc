//go:build unix

package main

import (
	"errors"
	"testing"
)

// **エンジンが 2 台になる道を、ここで塞いでいる。** 裸の `sshc` はサブコマンドの
// どれにも当たらないので、これが無ければそのまま 2 台目が上がる。flock は
// open file description ごとに効くので、同じプロセスから開き直した 2 本目でも
// 本物どおり弾かれる——別プロセスを起こさずに、この規則そのものを見られる。
func TestLockEngineStartRefusesASecondEngine(t *testing.T) {
	stateDir := t.TempDir()

	release, err := lockEngineStart(stateDir)
	if err != nil {
		t.Fatalf("the first engine could not take the lock: %v", err)
	}

	if _, err := lockEngineStart(stateDir); !errors.Is(err, errEngineRunning) {
		t.Fatalf("the second engine got %v, want errEngineRunning", err)
	}

	// **プロセスが死ねば必ず外れる。** 手放したあとは次の 1 台が上がれる
	// ——外れないロックは、誰もエンジンを起こせない状態を永久に残す。
	release()
	next, err := lockEngineStart(stateDir)
	if err != nil {
		t.Fatalf("the lock stayed held after it was released: %v", err)
	}
	next()
}

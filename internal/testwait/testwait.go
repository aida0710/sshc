// Package testwait は、テストがメモリ上の状態の変化を待つときの上限と待ち方である。
//
// docker、VPN、実サーバーのような外の相手を待つ上限は、待つ相手ごとの名前付き
// 定数のまま持ち、ここには含めない。
package testwait

import (
	"testing"
	"time"
)

// Limit は、メモリ上の状態が変わるのを待つ上限である。CI は全 package を -race で
// 一度に走らせるので、CPU が飽和すると、プロセスの終了や接続の状態を観測する
// goroutine が数秒止まることがある。条件が満たされればすぐに返るので、正常系は
// この上限のぶん遅くならない。
const Limit = 15 * time.Second

// pollInterval は、条件を見直す間隔である。
const pollInterval = time.Millisecond

// Until は、condition が真になるまで待つ。Limit を過ぎても真にならなければテストを落とす。
func Until(t testing.TB, condition func() bool) {
	t.Helper()
	if !Reached(condition) {
		t.Fatal("the condition never became true")
	}
}

// Reached は、condition が Limit までに真になったかを返す。落とす前に後始末を
// したいときや、何が足りなかったかを報告したいときに使う。
func Reached(condition func() bool) bool {
	deadline := time.Now().Add(Limit)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(pollInterval)
	}
	return false
}

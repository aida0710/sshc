package secret_test

import (
	"testing"
	"time"
)

// 手動のロックでもアイドルによる自動ロックでも、開いていた vault が閉じたら一度だけ
// 知らせる。閉じている vault をもう一度ロックしても知らせない。
func TestTheVaultTellsTheLockListenerEachTimeAnOpenVaultCloses(t *testing.T) {
	clock := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	locks := 0
	service.SetAfterLock(func() { locks++ })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	service.Lock()
	if locks != 1 {
		t.Fatalf("after an explicit lock the listener heard %d lock(s), want 1", locks)
	}
	service.Lock()
	if locks != 1 {
		t.Fatalf("locking a closed vault told the listener again (%d)", locks)
	}

	if err := service.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	service.SetIdleTimeout(15 * time.Minute)
	clock = clock.Add(16 * time.Minute)
	if service.Unlocked() {
		t.Fatal("the idle timeout did not lock the vault")
	}
	if locks != 2 {
		t.Fatalf("after the idle lock the listener heard %d lock(s), want 2", locks)
	}
}

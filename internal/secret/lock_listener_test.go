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

// Unlock は vault を開けたときだけ知らせる。知らされた側は、変更のロックを
// 放したあとで呼ばれるので、vault の鍵を使う操作をしてよい。
func TestTheVaultTellsTheUnlockListenerOnlyWhenUnlockOpensIt(t *testing.T) {
	clock := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	unlocks := 0
	service.SetAfterUnlock(func() {
		unlocks++
		if _, err := service.KeyedTravelDigest("digest"); err != nil {
			t.Errorf("the listener could not use the vault key: %v", err)
		}
	})

	if err := service.Unlock("not the " + passphrase); err == nil {
		t.Fatal("a wrong passphrase unlocked the vault")
	}
	if unlocks != 0 {
		t.Fatalf("a refused unlock told the listener (%d)", unlocks)
	}
	if err := service.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if unlocks != 1 {
		t.Fatalf("after unlocking the listener heard %d unlock(s), want 1", unlocks)
	}
}

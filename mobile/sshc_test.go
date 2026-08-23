package mobile

import (
	"errors"
	"strings"
	"testing"
)

// started はテスト終了時に engine lock が必ず解放されるよう Cleanup を登録する。
func started(t *testing.T) string {
	t.Helper()
	url, err := Start(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	return url
}

// Start が返した時点で、WebView から入口へ接続できることを検証する。
func TestStartReturnsAnEntranceThatIsAlreadyServing(t *testing.T) {
	url := started(t)
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("URL = %q, want a loopback entrance", url)
	}
	if !strings.Contains(url, "/#bootstrap=") {
		t.Errorf("URL = %q, want a one-time bootstrap fragment", url)
	}
}

// 同一プロセスで2台目の engine を開始できないことを検証する。
func TestStartRefusesASecondEngine(t *testing.T) {
	started(t)
	if _, err := Start(t.TempDir(), t.TempDir()); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("second Start = %v, want ErrAlreadyStarted", err)
	}
}

// Stop 後に同じプロセスで engine を再開できることを検証する。
func TestStopLetsTheNextStartSucceed(t *testing.T) {
	home, cache := t.TempDir(), t.TempDir()
	if _, err := Start(home, cache); err != nil {
		t.Fatalf("first Start = %v", err)
	}
	if err := Stop(); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	url, err := Start(home, cache)
	if err != nil {
		t.Fatalf("second Start = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	if url == "" {
		t.Error("the second Start announced no entrance")
	}
}

func TestStopWithoutStartIsAnError(t *testing.T) {
	if err := Stop(); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Stop = %v, want ErrNotStarted", err)
	}
}

// gomobile 境界で error の型情報が失われても、失敗理由の番号が維持されることを検証する。
func TestTheFailureKindSurvivesWhereTheErrorWouldNot(t *testing.T) {
	started(t)
	if _, err := Start(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("a second Start succeeded")
	}
	if got := LastStartFailureKind(); got != KindAlreadyStarted {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindAlreadyStarted)
	}
}

// Start 成功時に以前の失敗理由が消去されることを検証する。
func TestASuccessfulStartClearsTheLastFailure(t *testing.T) {
	started(t)
	if got := LastStartFailureKind(); got != KindNone {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindNone)
	}
}

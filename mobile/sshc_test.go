package mobile

import (
	"errors"
	"strings"
	"testing"
)

// started はテスト終了時に engine が必ず停止するよう Cleanup を登録する。
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

// Serviceの再生成でStartが重複しても、古いengineを置き換えて入口を返す。
func TestStartReplacesTheRunningEngine(t *testing.T) {
	home, cache := t.TempDir(), t.TempDir()
	first, err := Start(home, cache)
	if err != nil {
		t.Fatalf("first Start = %v", err)
	}
	second, err := Start(home, cache)
	if err != nil {
		t.Fatalf("second Start = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	if second == "" || second == first {
		t.Errorf("replacement entrance = %q, first = %q", second, first)
	}
	if got := LastStartFailureKind(); got != KindNone {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindNone)
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
	if _, err := Start("relative-home", t.TempDir()); err == nil {
		t.Fatal("Start accepted a relative private storage path")
	}
	if got := LastStartFailureKind(); got != KindStorageUnavailable {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindStorageUnavailable)
	}
}

// Start 成功時に以前の失敗理由が消去されることを検証する。
func TestASuccessfulStartClearsTheLastFailure(t *testing.T) {
	started(t)
	if got := LastStartFailureKind(); got != KindNone {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindNone)
	}
}

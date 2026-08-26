package mobile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"sshc/internal/app"
	"sshc/internal/storage"
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

func TestStartPrefersAnImmediateRunFailureOverAnAnnouncedEntrance(t *testing.T) {
	runFailure := errors.New("accept failed immediately")
	probeFailure := errors.New("the announced entrance is already closed")
	probeStarted := make(chan struct{})
	runReturned := make(chan struct{})
	url, err := startWith(t.TempDir(), t.TempDir(),
		func(_ context.Context, dependencies app.Dependencies, _ string) error {
			defer close(runReturned)
			if announceErr := dependencies.Announce(app.Readiness{
				Entrance: "http://127.0.0.1:43123/#bootstrap=test",
			}); announceErr != nil {
				return announceErr
			}
			// Startがentrance側を選びprobeを開始するまで、Runを終了させない。
			// これでselectの乱数に依存せずAnnounce→Serve失敗の順序を再現する。
			<-probeStarted
			return runFailure
		},
		func(context.Context, string) error {
			close(probeStarted)
			<-runReturned
			return probeFailure
		},
	)
	if url != "" {
		t.Fatalf("Start returned a dead entrance %q", url)
	}
	if !errors.Is(err, runFailure) || !errors.Is(err, probeFailure) {
		t.Fatalf("Start error = %v, want run and probe failures", err)
	}
	if got := LastStartFailureKind(); got != KindEngineStartFailed {
		t.Fatalf("LastStartFailureKind = %d, want %d", got, KindEngineStartFailed)
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
	if got := LastStartFailureDetail(); got != "" {
		t.Errorf("LastStartFailureDetail() = %q, want empty", got)
	}
	if got := LastStartFailureCode(); got != "none" {
		t.Errorf("LastStartFailureCode() = %q, want none", got)
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
	running.Lock()
	_ = fail(KindEngineStartFailed, errors.New("previous failure"))
	running.Unlock()
	started(t)
	if got := LastStartFailureKind(); got != KindNone {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, KindNone)
	}
	if got := LastStartFailureDetail(); got != "" {
		t.Errorf("LastStartFailureDetail() = %q, want empty", got)
	}
}

func TestStartFailureClassificationDistinguishesThePort(t *testing.T) {
	if got, _ := classifyStartFailure(errors.Join(app.ErrListen, errors.New("address in use"))); got != KindListenFailed {
		t.Errorf("listen failure kind = %d, want %d", got, KindListenFailed)
	}
	if got, _ := classifyStartFailure(errors.New("publish handoff")); got != KindEngineStartFailed {
		t.Errorf("generic failure kind = %d, want %d", got, KindEngineStartFailed)
	}
	if got, _ := classifyStartFailure(storage.ErrSymlinkPath); got != KindStorageUnavailable {
		t.Errorf("storage failure kind = %d, want %d", got, KindStorageUnavailable)
	}
}

func TestFailureCodesAreStableAndShareable(t *testing.T) {
	wants := map[int]string{
		KindNone:               "none",
		KindListenFailed:       "port_unavailable",
		KindStoppedEarly:       "engine_stopped_early",
		KindStorageUnavailable: "storage_unavailable",
		KindEngineStartFailed:  "engine_start_failed",
		999:                    "unknown",
	}
	for kind, want := range wants {
		running.Lock()
		running.lastKind = kind
		running.Unlock()
		if got := LastStartFailureCode(); got != want {
			t.Errorf("LastStartFailureCode() for %d = %q, want %q", kind, got, want)
		}
	}
	t.Cleanup(func() {
		running.Lock()
		running.lastKind = KindNone
		running.lastDetail = ""
		running.Unlock()
	})
}

func TestFailureDetailRedactsCredentialsAndControls(t *testing.T) {
	errorWithSecrets := errors.New("open https://alice:hunter2@example.test/#bootstrap=once\n" +
		"token=abc secret=def password: ghi passphrase=jkl access_key_id=ak123 " +
		"secret-access-key=cloud456 authorization: auth789 Bearer bearer-value")
	got := safeFailureDetail(errorWithSecrets)
	for _, secret := range []string{"alice", "hunter2", "once", "abc", "def", "ghi", "jkl", "ak123", "cloud456", "auth789", "bearer-value", "\n"} {
		if strings.Contains(got, secret) {
			t.Errorf("detail %q contains %q", got, secret)
		}
	}
	for _, marker := range []string{"[redacted]@", "bootstrap=[redacted]", "token=[redacted]"} {
		if !strings.Contains(got, marker) {
			t.Errorf("detail %q does not contain %q", got, marker)
		}
	}
}

func TestFailureDetailHasABoundedUTF8Length(t *testing.T) {
	got := safeFailureDetail(errors.New(strings.Repeat("界", maxFailureDetailBytes)))
	if !utf8.ValidString(got) {
		t.Fatalf("detail is not UTF-8: %q", got)
	}
	if len(got) > maxFailureDetailBytes {
		t.Errorf("detail length = %d, want <= %d", len(got), maxFailureDetailBytes)
	}
}

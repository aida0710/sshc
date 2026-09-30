//go:build !windows

package vpn

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// routeFileWithin は、read が timeout までに終わることを確かめる。
func routeFileWithin(t *testing.T, timeout time.Duration, read func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		read()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("経路の置き場所のファイルを読んだまま戻らない")
	}
}

// コンテナが経路の置き場所に置いた symlink、上限を超えるファイル、FIFO は、トンネルの
// 様子として読まない。FIFO でも待たない。
func TestTheTunnelStatusIgnoresWhatAContainerCannotHonestlyWrite(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"interface":"leaked"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, placed := range []struct {
		name  string
		place func(path string) error
	}{
		{"symlink", func(path string) error { return os.Symlink(outside, path) }},
		{"上限を超えるファイル", func(path string) error {
			return os.WriteFile(path, []byte(`{"interface":"`+strings.Repeat("a", maxStatusBytes)+`"}`), 0o600)
		}},
		{"FIFO", func(path string) error { return syscall.Mkfifo(path, 0o600) }},
	} {
		t.Run(placed.name, func(t *testing.T) {
			manager := New(t.TempDir(), 1000, nil)
			if err := prepareRouteDirectory(manager.routeDirectory("lab")); err != nil {
				t.Fatal(err)
			}
			if err := placed.place(filepath.Join(manager.routeDirectory("lab"), statusFileName)); err != nil {
				t.Fatal(err)
			}

			routeFileWithin(t, 5*time.Second, func() {
				if tunnel := manager.tunnelStatus("lab"); tunnel != (TunnelStatus{}) {
					t.Errorf("tunnelStatus = %+v", tunnel)
				}
			})
		})
	}
}

// 失敗の理由の置き場所が FIFO でも待たずに、分からない理由として返す。
func TestAFailureReasonThatIsAFIFOFallsBackWithoutWaiting(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	if err := prepareRouteDirectory(manager.routeDirectory("lab")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(manager.routeDirectory("lab"), failureFileName), 0o600); err != nil {
		t.Fatal(err)
	}

	routeFileWithin(t, 5*time.Second, func() {
		failure, ok := manager.routeFailure("lab", FailureUnknown).(*RouteFailure)
		if !ok || failure.Reason != FailureUnknown {
			t.Errorf("routeFailure = %v", failure)
		}
	})
}

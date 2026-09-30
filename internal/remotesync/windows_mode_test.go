package remotesync_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

// windowsModeFileSystem は、Go の Windows 実装と同じく、通常ファイルの権限を書き込み
// ビットの有無から 0666 か 0444 として返す。Unix の上で Windows のマシンを演じるための
// もので、storage の内部テストにも同じ型がある。storage の内部テストはこのパッケージを
// import できない（import の循環になる）ので、共有していない。
type windowsModeFileSystem struct {
	storage.OSFileSystem
}

func (windowsModeFileSystem) ReportsExecutableBit() bool { return false }

func (fileSystem windowsModeFileSystem) Lstat(path string) (fs.FileInfo, error) {
	info, err := fileSystem.OSFileSystem.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return info, err
	}
	return windowsModeFileInfo{FileInfo: info}, nil
}

type windowsModeFileInfo struct{ fs.FileInfo }

func (info windowsModeFileInfo) Mode() fs.FileMode {
	if info.FileInfo.Mode().Perm()&0o200 == 0 {
		return info.FileInfo.Mode().Type() | 0o444
	}
	return info.FileInfo.Mode().Type() | 0o666
}

func TestAWindowsMachineKeepsTheExecutableModeItReceived(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix machine in this test needs a real owner execute bit")
	}
	bucket := &fakeBucket{}
	unix := newInstallation(t, bucket, map[string]string{
		"config":   "Host bastion\n  ProxyCommand ~/.ssh/proxy.sh %h %p\n",
		"proxy.sh": "#!/bin/sh\n",
	})
	proxy := filepath.Join(unix.home, ".ssh", "proxy.sh")
	if err := os.Chmod(proxy, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.service.PushUsing(context.Background(), keyOf(syncPassphrase), "Add proxy"); err != nil {
		t.Fatal(err)
	}

	windows := newInstallationOn(t, bucket, map[string]string{}, windowsModeFileSystem{})
	receive(t, windows)
	// 自動同期は受信のあと同じ比較で送信の要否を決める。ここで差分があれば、
	// 0600 に落ちた proxy.sh を送り返してしまう。
	if _, err := windows.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); !errors.Is(err, remotesync.ErrNothingToPush) {
		t.Fatalf("Push right after receiving = %v, want ErrNothingToPush", err)
	}

	windows.write(t, "config", "Host bastion\n  ProxyCommand ~/.ssh/proxy.sh %h %p\n  User deploy\n")
	if _, err := windows.service.PushUsing(context.Background(), keyOf(syncPassphrase), "Set user"); err != nil {
		t.Fatal(err)
	}
	receive(t, unix)
	if got := unix.read(t, "config"); got != "Host bastion\n  ProxyCommand ~/.ssh/proxy.sh %h %p\n  User deploy\n" {
		t.Fatalf("config = %q", got)
	}
	info, err := os.Lstat(proxy)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("proxy.sh mode after the Windows push = %04o, want 0700", info.Mode().Perm())
	}
}

// receive は、リモートの変更を競合なしに取得して適用する。
func receive(t *testing.T, machine installation) {
	t.Helper()
	preview, err := machine.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPreview(machine.service, remotesync.ResolveNone, "", preview); err != nil {
		t.Fatal(err)
	}
}

func TestAWindowsMachineSendsANewFileAsNotExecutable(t *testing.T) {
	bucket := &fakeBucket{}
	windows := newInstallationOn(t, bucket, map[string]string{"config": "Host bastion\n"}, windowsModeFileSystem{})

	manifest, _, err := windows.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Files {
		if entry.Path == "config" && entry.Mode != "0600" {
			t.Fatalf("config mode = %q, want 0600", entry.Mode)
		}
	}
}

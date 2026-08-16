//go:build unix

package keys

import (
	"os"
	"testing"
)

// このファイルは、Unix の mode ビットでしか言えない約束をまとめて持つ。
//
// Windows の Chmod が写すのは所有者の書き込みビットひとつだけで、Perm() は 0666
// か 0444 のどちらかにしかならない。向こうで誰が読めるかを決めているのは DACL
// であり、対になる filemode_windows_test.go がそちらを確かめる。同じ検査を両方で
// 走らせると、落ちるだけでなく「ここにアクセス制御がある」という嘘が残る。

func assertScannedPermission(t *testing.T, item *Item, want string) {
	t.Helper()
	if item.Permission != want {
		t.Errorf("Permission = %q, want %q", item.Permission, want)
	}
}

func assertPrivateKeyPermissionRisk(t *testing.T, exposed, safe *Item) {
	t.Helper()
	if !exposed.PermissionRisk {
		t.Errorf("a world-readable private key was not flagged")
	}
	if safe.PermissionRisk {
		t.Errorf("a 0600 private key was flagged as risky")
	}
}

func assertGeneratedKeyIsPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("%s permission = %04o, want 0600", path, info.Mode().Perm())
	}
}

func tightenTrashSourceKey(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("tighten permissions: %v", err)
	}
}

func assertTrashEntryIsPrivate(t *testing.T, entryDirectory, keyPath string) {
	t.Helper()
	directoryInfo, err := os.Lstat(entryDirectory)
	if err != nil {
		t.Fatalf("trash entry missing: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Errorf("trash directory permission = %04o, want 0700", directoryInfo.Mode().Perm())
	}
	keyInfo, err := os.Lstat(keyPath)
	if err != nil {
		t.Fatalf("trashed key missing: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o400 {
		t.Errorf("trashed key permission = %04o, want the original 0400", keyInfo.Mode().Perm())
	}
}

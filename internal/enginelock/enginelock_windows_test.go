//go:build windows

package enginelock

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowsacl"
	"sshc/internal/platform/windowsacl/acltest"
)

// ロックファイルと state ディレクトリは、作成した時点で現在のユーザー所有かつ
// 保護 DACL でなければならない。あとから締めるのでは、その隙間に別のユーザーが開ける。
func TestWindowsEngineLockStateIsPrivateAndNotAReparsePoint(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire = %v", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			t.Fatal(releaseErr)
		}
	}()

	for _, target := range []string{filepath.Dir(path), path} {
		restricted, checkErr := windowsacl.IsRestrictedToCurrentUser(target)
		if checkErr != nil {
			t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", target, checkErr)
		}
		if !restricted {
			t.Fatalf("%q is not restricted to the current user", target)
		}
		info, statErr := os.Lstat(target)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			t.Fatalf("%q is a reparse point", target)
		}
	}
}

// 別のユーザーが所有するロックファイルは、ロックを取る前に拒否しなければならない。
// 受け入れてしまえば、所有権の直列化を別のユーザーの書ける状態に委ねることになる。
func TestWindowsEngineLockRefusesAForeignOwnerFile(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	acltest.InstallForeignOwner(t, path)

	second, err := Acquire(path)
	if !errors.Is(err, windowsacl.ErrUnexpectedOwner) {
		t.Fatalf("Acquire on a foreign-owner lock = %v, want ErrUnexpectedOwner", err)
	}
	if second != nil {
		t.Fatal("a refused Acquire returned a release function")
	}
}

func TestWindowsEngineLockRefusesAJunctionedStateDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "state")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create privilege-free junction fixture: %v: %s", err, output)
	}

	release, err := Acquire(filepath.Join(junction, "engine.lock"))
	if !errors.Is(err, windowsacl.ErrReparsePoint) {
		t.Fatalf("Acquire through a junctioned state directory = %v, want ErrReparsePoint", err)
	}
	if release != nil {
		t.Fatal("a refused Acquire returned a release function")
	}
}

func TestWindowsEngineLockPinsTheDirectoryBeforeAPathReplacement(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "state")
	path := filepath.Join(directory, "engine.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	// The contention contract is covered independently. Keeping the byte-range
	// lock here would prevent Windows from renaming its parent directory, so it
	// would test a filesystem restriction instead of the handle-relative open.
	if err := first(); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(root, "state-original")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	afterLockDirectoryOpen = func() {
		afterLockDirectoryOpen = nil
		if renameErr := os.Rename(directory, moved); renameErr != nil {
			t.Fatal(renameErr)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", directory, outside).CombinedOutput(); junctionErr != nil {
			t.Fatalf("replace state with junction: %v: %s", junctionErr, output)
		}
	}
	t.Cleanup(func() { afterLockDirectoryOpen = nil })

	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire across directory replacement = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "engine.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replacement target received a lock file: %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(moved, "engine.lock")); statErr != nil {
		t.Fatalf("pinned directory lost its lock file: %v", statErr)
	}
}

// installForeignOwnerExactDACL は、現在のトークンが持つ別の owner SID を使って、
// 「自分では所有していないが自分は読み書きできる」状態を作る。管理者権限は要らない。

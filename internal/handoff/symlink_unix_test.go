//go:build unix

package handoff_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/platform/nofollow"
)

// state directory が symlink に差し替えられていたら、その先のディレクトリの mode を
// 変えず、lock file も作らずに断る。置き換えだけが symlink を拒んでいると、断る前に
// symlink の先で chmod と lock file の作成が済んでしまう。
func TestWriteRefusesASymlinkedStateDirectoryWithoutTouchingItsTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "elsewhere")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "sshc")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := handoff.Write(link, validDocument()); !errors.Is(err, nofollow.ErrSymlinkPath) {
		t.Fatalf("Write through a symlinked directory = %v, want ErrSymlinkPath", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("symlink target mode = %o, want it left at 0755", info.Mode().Perm())
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("symlink target entries = %v, want nothing created there", entries)
	}
}

// cli が別の場所への symlink なら、Read も Remove もその先を読まない。Remove は
// symlink もその先も消さない。
func TestReadAndRemoveRefuseASymlinkedHandoffFile(t *testing.T) {
	directory := t.TempDir()
	elsewhere := t.TempDir()
	document := validDocument()
	if err := handoff.Write(elsewhere, document); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(elsewhere, handoff.FileName)
	link := filepath.Join(directory, handoff.FileName)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if _, err := handoff.Read(directory); !errors.Is(err, nofollow.ErrSymlinkPath) {
		t.Fatalf("Read through a symlinked cli = %v, want ErrSymlinkPath", err)
	}
	if err := handoff.Remove(directory, document.Secret); !errors.Is(err, nofollow.ErrSymlinkPath) {
		t.Fatalf("Remove through a symlinked cli = %v, want ErrSymlinkPath", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("Remove deleted the symlink: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("Remove deleted the symlink target: %v", err)
	}
}

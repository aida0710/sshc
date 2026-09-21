package handoff

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

// failingFileSystem は OS の file system のうち 1 つの操作だけを失敗させる。
// interface として包むので storage の native な原子的置き換えは選ばれず、
// WriteTemp／Rename／SyncDir を順に呼ぶ経路を通る。どの段階で止まっても文書が
// 公開されないことを見られる。
type failingFileSystem struct {
	storage.FileSystem
	writeTemp error
	rename    error
	syncDir   error
}

func (f failingFileSystem) WriteTemp(directory, prefix string, permission fs.FileMode, contents []byte) (string, error) {
	if f.writeTemp != nil {
		return "", f.writeTemp
	}
	return f.FileSystem.WriteTemp(directory, prefix, permission, contents)
}

func (f failingFileSystem) Rename(oldPath, newPath string) error {
	if f.rename != nil {
		return f.rename
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f failingFileSystem) SyncDir(path string) error {
	if f.syncDir != nil {
		return f.syncDir
	}
	return f.FileSystem.SyncDir(path)
}

func writeWith(t *testing.T, directory string, fileSystem storage.FileSystem) error {
	t.Helper()
	operations := defaultWriteOperations()
	operations.fileSystem = fileSystem
	return write(directory, Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret that must not leak through a failed write",
		Owner:           OwnerEngine,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}, operations)
}

func TestWriteRemovesTheTemporaryFileWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	renameFailure := errors.New("rename failed")

	err := writeWith(t, directory, failingFileSystem{FileSystem: storage.OSFileSystem{}, rename: renameFailure})
	if !errors.Is(err, renameFailure) {
		t.Fatalf("Write = %v, want rename failure", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), temporaryPrefix) {
			t.Errorf("temporary file was left behind: %q", entry.Name())
		}
	}
}

func TestWriteDoesNotPublishWhenPrivateTempCreationFails(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("private temp creation failed")

	err := writeWith(t, directory, failingFileSystem{FileSystem: storage.OSFileSystem{}, writeTemp: want})
	if !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == FileName || strings.HasPrefix(entry.Name(), temporaryPrefix) {
			t.Fatalf("failed private creation published %q", entry.Name())
		}
	}
}

func TestWriteReportsTheDirectoryDurabilityFailureAfterReplacement(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("directory durability failed")

	err := writeWith(t, directory, failingFileSystem{FileSystem: storage.OSFileSystem{}, syncDir: want})
	if !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	if _, err := os.Stat(filepath.Join(directory, FileName)); err != nil {
		t.Fatalf("replacement did not precede directory durability: %v", err)
	}
}

package storage

import (
	"bytes"
	"io/fs"
	"testing"
)

// windowsModeFileSystem は、Go の Windows 実装と同じく、通常ファイルの権限を書き込み
// ビットの有無から 0666 か 0444 として返す。remotesync のテストにも同じ型がある。
// この内部テストは storage を import するパッケージを import できない（import の循環に
// なる）ので、共有していない。
type windowsModeFileSystem struct {
	OSFileSystem
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

func TestCommitWithoutExecutableBitsAcceptsAPreconditionOnAnExecutableFile(t *testing.T) {
	workspace := newTestWorkspace(t)
	target := writeWorkspaceFile(t, workspace, "proxy.sh", "#!/bin/sh\n", DirectoryPermission)
	workspace.fileSystem = windowsModeFileSystem{}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))

	if _, err := manager.Commit(Request{
		Operation: "sync.pull",
		Changes: []Change{{
			Path: target, Contents: []byte("#!/bin/sh\nexec nc %h %p\n"), Mode: DirectoryPermission,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("#!/bin/sh\n")), Mode: DirectoryPermission},
		}},
	}); err != nil {
		t.Fatalf("Commit = %v; the unreadable execute bit was taken for a concurrent change", err)
	}
	assertFileContents(t, target, "#!/bin/sh\nexec nc %h %p\n")
}

func TestRecoveryWithoutExecutableBitsCompletesAnAppliedExecutableWrite(t *testing.T) {
	workspace := newTestWorkspace(t)
	script := writeWorkspaceFile(t, workspace, "proxy.sh", "before\n", FilePermission)
	second := writeWorkspaceFile(t, workspace, "second.conf", "before\n", FilePermission)
	id := commitWithStaleJournal(t, workspace, 0x84, Request{
		Operation: "sync.pull",
		Changes: []Change{
			{
				Path: script, Contents: []byte("after\n"), Mode: DirectoryPermission,
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n")), Mode: FilePermission},
			},
			{
				Path: second, Contents: []byte("after\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n")), Mode: FilePermission},
			},
		},
	})
	workspace.fileSystem = windowsModeFileSystem{}

	restarted := restartedManager(t, workspace)
	reconciledPending(t, restarted, id, 1)
	if err := restarted.Complete(id); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, script, "after\n")
	assertFileContents(t, second, "after\n")
}

func TestRecoveryWithoutExecutableBitsCompletesAStagedExecutableWrite(t *testing.T) {
	workspace := newTestWorkspace(t)
	first := writeWorkspaceFile(t, workspace, "first.conf", "before\n", FilePermission)
	script := writeWorkspaceFile(t, workspace, "proxy.sh", "before\n", FilePermission)
	id := commitWithStaleJournal(t, workspace, 0x85, Request{
		Operation: "sync.pull",
		Changes: []Change{
			{
				Path: first, Contents: []byte("after\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n")), Mode: FilePermission},
			},
			{
				Path: script, Contents: []byte("after\n"), Mode: DirectoryPermission,
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n")), Mode: FilePermission},
			},
		},
	})
	workspace.fileSystem = windowsModeFileSystem{}

	restarted := restartedManager(t, workspace)
	reconciledPending(t, restarted, id, 1)
	if err := restarted.Complete(id); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, first, "after\n")
	assertFileContents(t, script, "after\n")
}

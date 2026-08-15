//go:build windows

package keys

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
	"sshc/internal/storage"
)

func TestTrashAndRestoreAuthenticateWindowsPrivateMoves(t *testing.T) {
	for _, test := range []struct {
		name       string
		permissive bool
	}{
		{name: "initially permissive source", permissive: true},
		{name: "already restricted source", permissive: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, root := newTrashService(t)
			originals := []string{
				filepath.Join(root, "id_work"),
				filepath.Join(root, "id_work.pub"),
			}
			for _, path := range originals {
				if test.permissive {
					installPermissiveKeyDACL(t, path)
				} else {
					assertRestrictedKeyPath(t, path)
				}
			}

			trashed, err := service.Trash(ItemID("id_work"))
			if err != nil {
				t.Fatalf("Trash = %v", err)
			}
			for _, file := range trashed.Files {
				assertRestrictedKeyPath(t, filepath.Join(root, file.TrashRelativePath))
			}

			restored, err := service.Restore(trashed.EntryID)
			if err != nil {
				t.Fatalf("Restore = %v", err)
			}
			if len(restored.Restored) != len(originals) {
				t.Fatalf("Restore restored %d files, want %d", len(restored.Restored), len(originals))
			}
			for _, path := range originals {
				assertRestrictedKeyPath(t, path)
			}
		})
	}
}

func TestTrashRollbackRestoresTheAuthenticatedWindowsHandle(t *testing.T) {
	wantFailure := errors.New("fail the second private move")
	fileSystem := &failingWindowsPrivateMoveFileSystem{
		FileSystem: storage.OSFileSystem{},
		failure:    wantFailure,
	}
	service, workspace, manager := newWindowsTrashService(t, fileSystem)
	originals := []string{
		filepath.Join(workspace.Root(), "id_work"),
		filepath.Join(workspace.Root(), "id_work.pub"),
	}
	for _, path := range originals {
		installPermissiveKeyDACL(t, path)
	}

	fileSystem.failAt = 2
	fileSystem.enabled = true
	if _, err := service.Trash(ItemID("id_work")); !errors.Is(err, wantFailure) {
		t.Fatalf("Trash = %v, want injected failure", err)
	}
	if len(fileSystem.succeeded) != 1 {
		t.Fatalf("successful private moves = %#v, want one before failure", fileSystem.succeeded)
	}
	moved := fileSystem.succeeded[0]
	if _, err := os.Stat(moved.from); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed source %q still exists: %v", moved.from, err)
	}
	assertRestrictedKeyPath(t, moved.to)

	pending, err := manager.Pending()
	if err != nil {
		t.Fatalf("Pending = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Pending = %#v, want one interrupted Trash", pending)
	}
	if err := manager.Rollback(pending[0].ID); err != nil {
		t.Fatalf("Rollback = %v", err)
	}
	assertRestrictedKeyPath(t, moved.from)
	if _, err := os.Stat(moved.to); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back trash path %q still exists: %v", moved.to, err)
	}
}

type windowsPrivateMove struct {
	from string
	to   string
}

type failingWindowsPrivateMoveFileSystem struct {
	storage.FileSystem
	enabled   bool
	failAt    int
	calls     int
	failure   error
	succeeded []windowsPrivateMove
}

func (fileSystem *failingWindowsPrivateMoveFileSystem) MovePrivate(oldPath, newPath string) error {
	if !fileSystem.enabled {
		return fileSystem.FileSystem.MovePrivate(oldPath, newPath)
	}
	fileSystem.calls++
	if fileSystem.calls == fileSystem.failAt {
		fileSystem.enabled = false
		return fileSystem.failure
	}
	if err := fileSystem.FileSystem.MovePrivate(oldPath, newPath); err != nil {
		return err
	}
	fileSystem.succeeded = append(fileSystem.succeeded, windowsPrivateMove{from: oldPath, to: newPath})
	return nil
}

func newWindowsTrashService(t *testing.T, fileSystem storage.FileSystem) (*Service, *storage.Workspace, *storage.Manager) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("create SSH directory: %v", err)
	}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatalf("NewWorkspace = %v", err)
	}
	clock := steppingClock(time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC))
	manager := storage.NewManager(workspace, clock, rand.Reader)
	manager.Seal = sealForTest
	manager.Unseal = unsealForTest
	service := NewService(ServiceOptions{
		Workspace:    workspace,
		Transactions: manager,
		Resolver:     storage.NewResolver(workspace),
		Catalogue:    CatalogueReader{Toolchain: fakeToolchain{}},
		Now:          clock,
		Random:       rand.Reader,
	})
	if _, err := service.Generate(GenerateRequest{
		Algorithm:  AlgorithmEd25519,
		FileName:   "id_work",
		Comment:    "aida@laptop",
		Passphrase: []byte("correct horse"),
	}); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	return service, workspace, manager
}

func installPermissiveKeyDACL(t *testing.T, path string) {
	t.Helper()
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:(A;;FA;;;" + user.User.Sid.String() + ")(A;;GR;;;BU)",
	)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("permissive key DACL = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(user)
	if restricted, err := windowsacl.IsRestrictedToCurrentUser(path); err != nil {
		t.Fatal(err)
	} else if restricted {
		t.Fatalf("permissive fixture %q was reported restricted", path)
	}
}

func assertRestrictedKeyPath(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("%q does not have the exact private Windows DACL", path)
	}
}

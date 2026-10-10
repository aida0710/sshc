//go:build !windows

package sftp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalOperationsIdentifyPermissionRefusalOnTheEngine(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	operations := []struct {
		name string
		run  func(*TransferManager, LocalDeleteEntry) error
	}{
		{"mkdir", func(manager *TransferManager, selection LocalDeleteEntry) error {
			_, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: filepath.Dir(selection.Path), Name: "new"})
			return err
		}},
		{"rename", func(manager *TransferManager, selection LocalDeleteEntry) error {
			_, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: selection.Path, Name: "new", ExpectedRevision: selection.ExpectedRevision})
			return err
		}},
		{"delete confirmation", func(manager *TransferManager, selection LocalDeleteEntry) error {
			plan, err := manager.PrepareLocalDelete(t.Context(), []LocalDeleteEntry{selection})
			if plan != nil {
				plan.Close()
			}
			return err
		}},
		{"comparison", func(_ *TransferManager, selection LocalDeleteEntry) error {
			_, err := readLocalComparisonTree(t.Context(), filepath.Dir(selection.Path))
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "keep.txt")
			writeLocalMutationFile(t, filename, "keep")
			selection := localDeleteSelectionForTest(t, filename)[0]
			if err := os.Chmod(directory, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
			if err := operation.run(manager, selection); !errors.Is(err, ErrLocalPermissionDenied) || !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("operation = %v, want a local permission refusal", err)
			}
		})
	}
}

func TestLocalDeleteRefusesRevokedPermissionsWithoutRemovingTheFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "keep.txt")
	writeLocalMutationFile(t, filename, "keep")
	plan, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, filename))
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if err := os.Chmod(directory, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	if err := plan.Delete(t.Context(), plan.Revision); !errors.Is(err, ErrLocalPermissionDenied) {
		t.Fatalf("delete = %v, want a local permission refusal", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, filename, "keep")
}

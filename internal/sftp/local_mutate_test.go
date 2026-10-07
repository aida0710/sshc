package sftp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func localMutationFixture(t *testing.T) (*TransferManager, string) {
	t.Helper()
	fixture := t.TempDir()
	home := filepath.Join(fixture, "home")
	directory := filepath.Join(fixture, "files")
	for _, value := range []string{home, directory} {
		if err := os.Mkdir(value, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	manager := NewTransferManager(&Service{}, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })
	return manager, directory
}

func localDeleteSelectionForTest(t *testing.T, paths ...string) []LocalDeleteEntry {
	t.Helper()
	selection := make([]LocalDeleteEntry, len(paths))
	for index, value := range paths {
		metadata, err := os.Lstat(value)
		if err != nil {
			t.Fatal(err)
		}
		selection[index] = LocalDeleteEntry{Path: filepath.ToSlash(value), ExpectedRevision: metadataRevision(metadata)}
	}
	return selection
}

func writeLocalMutationFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertLocalMutationFile(t *testing.T, filename, contents string) {
	t.Helper()
	actual, err := os.ReadFile(filename)
	if err != nil || string(actual) != contents {
		t.Fatalf("%s = %q, %v; want %q", filename, actual, err, contents)
	}
}

func symlinkLocalMutationFixture(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func TestLocalMkdirAndRenamePreserveAnExistingDestination(t *testing.T) {
	manager, directory := localMutationFixture(t)
	created, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: directory, Name: "new folder"})
	if err != nil || created.Type != EntryDirectory || created.Path != filepath.ToSlash(filepath.Join(directory, "new folder")) {
		t.Fatalf("mkdir = %+v, %v", created, err)
	}
	if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: directory, Name: "new folder"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("repeat mkdir = %v", err)
	}
	source := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, source, "notes")
	revision := localDeleteSelectionForTest(t, source)[0].ExpectedRevision
	renamed, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: source, Name: "renamed.txt", ExpectedRevision: revision})
	if err != nil || renamed.Path != filepath.ToSlash(filepath.Join(directory, "renamed.txt")) {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	writeLocalMutationFile(t, source, "keep")
	if _, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: renamed.Path, Name: "notes.txt", ExpectedRevision: renamed.Revision}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("rename onto existing = %v", err)
	}
	assertLocalMutationFile(t, source, "keep")
	assertLocalMutationFile(t, filepath.FromSlash(renamed.Path), "notes")
}

func TestLocalMutationsRejectTraversalReservedNamesAndProtectedRoots(t *testing.T) {
	manager, directory := localMutationFixture(t)
	for _, name := range []string{"", ".", "..", "../outside", `a\b`, "a/b", "nul\x00", ".sshc-download-test", ".a.sshc-upload-abcdefgh.part", ".a.sshc-" + strings.Repeat("a", 24) + ".tmp"} {
		if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: directory, Name: name}); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("mkdir %q = %v", name, err)
		}
	}
	home, _ := os.UserHomeDir()
	root := filepath.VolumeName(directory) + string(filepath.Separator)
	for _, value := range []string{home, filepath.Dir(home), root} {
		_, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, value))
		if !errors.Is(err, ErrRootOperation) {
			t.Errorf("delete protected %q = %v", value, err)
		}
	}
	for _, value := range []string{"~/../files", directory + "/../files", "relative/file", directory + "//file"} {
		if _, err := cleanLocalMutationPath(value); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("path %q = %v", value, err)
		}
		if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: value, Name: "folder"}); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("mkdir parent %q = %v", value, err)
		}
	}
}

func TestLocalDeleteInspectsAllSelectionsAndNeverFollowsLinks(t *testing.T) {
	manager, directory := localMutationFixture(t)
	outside := filepath.Join(directory, "keep.txt")
	writeLocalMutationFile(t, outside, "outside")
	folder, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: directory, Name: "folder"})
	if err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, "folder", "notes.txt"), "inside")
	symlinkLocalMutationFixture(t, outside, filepath.Join(directory, "folder", "link"))
	broken := filepath.Join(directory, "broken")
	symlinkLocalMutationFixture(t, filepath.Join(directory, "missing"), broken)
	selection := localDeleteSelectionForTest(t, filepath.FromSlash(folder.Path), broken)
	plan, err := manager.PrepareLocalDelete(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if plan.Items != 4 {
		t.Fatalf("items = %d", plan.Items)
	}
	if err := plan.Delete(t.Context(), plan.Revision); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{folder.Path, broken} {
		if _, err := os.Lstat(filepath.FromSlash(value)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("still present: %s, %v", value, err)
		}
	}
	assertLocalMutationFile(t, outside, "outside")
}

func TestLocalDeleteRefusesChangedDescendantsBeforeRemovingAnySelection(t *testing.T) {
	for _, change := range []string{"contents", "new child", "directory replaced by link", "same metadata replacement"} {
		t.Run(change, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			folder := filepath.Join(directory, "folder")
			if err := os.Mkdir(folder, 0o700); err != nil {
				t.Fatal(err)
			}
			child := filepath.Join(folder, "notes.txt")
			writeLocalMutationFile(t, child, "before")
			other := filepath.Join(directory, "aaa.txt")
			writeLocalMutationFile(t, other, "keep")
			plan, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, other, folder))
			if err != nil {
				t.Fatal(err)
			}
			defer plan.Close()
			switch change {
			case "contents":
				writeLocalMutationFile(t, child, "changed contents")
			case "new child":
				before, err := os.Stat(folder)
				if err != nil {
					t.Fatal(err)
				}
				writeLocalMutationFile(t, filepath.Join(folder, "new.txt"), "new")
				if err := os.Chtimes(folder, before.ModTime(), before.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "directory replaced by link":
				if err := os.Rename(folder, folder+"-old"); err != nil {
					t.Fatal(err)
				}
				symlinkLocalMutationFixture(t, directory, folder)
			case "same metadata replacement":
				before, err := os.Stat(child)
				if err != nil {
					t.Fatal(err)
				}
				// Keep the original inode allocated to avoid inode reuse.
				if err := os.Rename(child, child+"-old"); err != nil {
					t.Fatal(err)
				}
				writeLocalMutationFile(t, child, "before")
				if err := os.Chtimes(child, before.ModTime(), before.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			if err := plan.Delete(t.Context(), plan.Revision); !errors.Is(err, ErrConflict) {
				t.Fatalf("delete after %s = %v", change, err)
			}
			assertLocalMutationFile(t, other, "keep")
		})
	}
}

func TestLocalDeleteRefusesTemporaryFilesOverlapsAndCancelledPlans(t *testing.T) {
	manager, directory := localMutationFixture(t)
	folder := filepath.Join(directory, "folder")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(folder, "keep.txt")
	writeLocalMutationFile(t, keep, "keep")
	selection := localDeleteSelectionForTest(t, folder)
	plan, err := manager.PrepareLocalDelete(t.Context(), append(selection, localDeleteSelectionForTest(t, keep)...))
	if plan != nil {
		plan.Close()
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("overlap = %v", err)
	}
	temporary := filepath.Join(folder, ".sshc-download-active")
	writeLocalMutationFile(t, temporary, "part")
	if _, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, folder)); !errors.Is(err, ErrConflict) {
		t.Fatalf("temporary = %v", err)
	}
	if _, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: folder, Name: "renamed", ExpectedRevision: localDeleteSelectionForTest(t, folder)[0].ExpectedRevision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("rename folder with temporary = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := manager.PrepareLocalDelete(ctx, localDeleteSelectionForTest(t, keep)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled prepare = %v", err)
	}
	plan, err = manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, keep))
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if err := plan.Delete(ctx, plan.Revision); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled delete = %v", err)
	}
	assertLocalMutationFile(t, keep, "keep")
}

func TestLocalDeleteTraversalLimitRemovesNothing(t *testing.T) {
	manager, directory := localMutationFixture(t)
	keep := filepath.Join(directory, "aaa.txt")
	writeLocalMutationFile(t, keep, "keep")
	folder := filepath.Join(directory, "tree")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	relative := ""
	for range maxDeleteDepth + 1 {
		relative += "d/"
		if err := root.Mkdir(relative, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, keep, folder)); !errors.Is(err, ErrTraversalLimit) {
		t.Fatalf("deep delete = %v", err)
	}
	assertLocalMutationFile(t, keep, "keep")
	if _, err := os.Stat(folder); err != nil {
		t.Fatal(err)
	}
}

func TestLocalRenameAndMkdirPinASymlinkParentAndActOnTheLinkItself(t *testing.T) {
	manager, directory := localMutationFixture(t)
	symlinkLocalMutationFixture(t, directory, filepath.Join(filepath.Dir(directory), "shortcut"))
	shortcut := filepath.Join(filepath.Dir(directory), "shortcut")
	if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: shortcut, Name: "folder"}); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(directory, "keep.txt")
	writeLocalMutationFile(t, keep, "keep")
	link := filepath.Join(shortcut, "link")
	symlinkLocalMutationFixture(t, keep, link)
	selection := localDeleteSelectionForTest(t, link)
	if _, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: link, Name: "renamed-link", ExpectedRevision: selection[0].ExpectedRevision}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(directory, "renamed-link")); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, keep, "keep")
}

func TestLocalMutationsProtectTransferPathsIncludingTheirParents(t *testing.T) {
	for _, operation := range []RemoteTransferOperation{RemoteGet, RemotePut} {
		t.Run(string(operation), func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			child := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, child, "keep")
			// A paused job retains its source or destination for resumption.
			job := TransferJob{Operation: operation, Status: TransferPaused, SourcePath: child, RemotePath: child}
			manager.jobs["local_test_transfer"] = &transferJobRecord{job: job}
			for _, value := range []string{directory, child} {
				if _, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, value)); !errors.Is(err, ErrConflict) {
					t.Fatalf("delete active transfer %s = %v", value, err)
				}
			}
			if _, err := manager.RenameLocal(t.Context(), LocalRenameRequest{Path: child, Name: "renamed", ExpectedRevision: localDeleteSelectionForTest(t, child)[0].ExpectedRevision}); !errors.Is(err, ErrConflict) {
				t.Fatalf("rename active transfer = %v", err)
			}
			job.SourcePath, job.RemotePath = filepath.Join(directory, "new"), filepath.Join(directory, "new")
			manager.jobs["local_test_transfer"].job = job
			if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Directory: directory, Name: "new"}); !errors.Is(err, ErrConflict) {
				t.Fatalf("mkdir transfer destination = %v", err)
			}
			assertLocalMutationFile(t, child, "keep")
		})
	}
}

func TestLocalDeletePlanDetectsAnIdentityChangeEvenWithTheSameMetadata(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "same")
	metadata, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	selection := localDeleteSelectionForTest(t, filename)
	before, err := manager.PrepareLocalDelete(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	revision := before.Revision
	before.Close()
	if err := os.Rename(filename, filename+"-old"); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filename, "same")
	if err := os.Chtimes(filename, metadata.ModTime(), metadata.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := manager.PrepareLocalDelete(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if revision == after.Revision {
		t.Fatal("a replacement inode reused the confirmation revision")
	}
	if err := after.Delete(t.Context(), revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale plan = %v", err)
	}
	assertLocalMutationFile(t, filename, "same")
}

func TestLocalDeleteSelectionLimitAndMissingRevisionRemoveNothing(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "keep")
	selection := localDeleteSelectionForTest(t, filename)
	tooMany := make([]LocalDeleteEntry, maxLocalDeleteSelection+1)
	for index := range tooMany {
		tooMany[index] = selection[0]
	}
	for _, entries := range [][]LocalDeleteEntry{nil, tooMany, {{Path: filename}}} {
		if plan, err := manager.PrepareLocalDelete(t.Context(), entries); err == nil {
			plan.Close()
			t.Fatal("invalid selection was accepted")
		}
	}
	if _, err := manager.MkdirLocal(t.Context(), LocalMkdirRequest{Name: "folder"}); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("missing parent = %v", err)
	}
	assertLocalMutationFile(t, filename, "keep")
}

func TestLocalDeleteRefusesUnreadableTreesWithoutRemovingOtherSelections(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("fixture requires Unix permissions and an unprivileged user")
	}
	manager, directory := localMutationFixture(t)
	keep := filepath.Join(directory, "aaa.txt")
	writeLocalMutationFile(t, keep, "keep")
	private := filepath.Join(directory, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	selection := localDeleteSelectionForTest(t, keep, private)
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o700) })
	selection[1].ExpectedRevision = localDeleteSelectionForTest(t, private)[0].ExpectedRevision
	if _, err := manager.PrepareLocalDelete(t.Context(), selection); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable tree = %v", err)
	}
	assertLocalMutationFile(t, keep, "keep")
}

func TestNativeLocalRenameNeverReplacesAnExistingDestination(t *testing.T) {
	_, directory := localMutationFixture(t)
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeLocalMutationFile(t, filepath.Join(directory, "source"), "source")
	writeLocalMutationFile(t, filepath.Join(directory, "destination"), "destination")
	if err := renameLocalMutationWithoutReplace(root, "source", "destination"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("rename = %v", err)
	}
	assertLocalMutationFile(t, filepath.Join(directory, "source"), "source")
	assertLocalMutationFile(t, filepath.Join(directory, "destination"), "destination")
}

func TestLocalDeleteProtectsACancelledTransferUntilItsWorkerHasStopped(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "keep")
	manager.jobs["local_test_transfer"] = &transferJobRecord{job: TransferJob{Operation: RemotePut, Status: TransferCancelled, SourcePath: filename}}
	ctx, cancel := context.WithCancel(t.Context())
	run := &remoteRun{ctx: ctx, cancel: cancel}
	manager.remoteRuns["local_test_transfer"] = run
	if err := manager.protectLocalTransferRun(run, manager.jobs["local_test_transfer"].job); err != nil {
		t.Fatal(err)
	}
	cancel()
	selection := localDeleteSelectionForTest(t, filename)
	if _, err := manager.PrepareLocalDelete(t.Context(), selection); !errors.Is(err, ErrConflict) {
		t.Fatalf("worker still present = %v", err)
	}
	delete(manager.remoteRuns, "local_test_transfer")
	delete(manager.jobs, "local_test_transfer")
	if _, err := manager.PrepareLocalDelete(t.Context(), selection); !errors.Is(err, ErrConflict) {
		t.Fatalf("removed job still has a worker = %v", err)
	}
	delete(manager.localTransferRuns, run)
	plan, err := manager.PrepareLocalDelete(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	plan.Close()
	assertLocalMutationFile(t, filename, "keep")
}

func TestLocalTransferCancelledDuringInspectionDoesNotAcquireItsPath(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "keep")
	plan, err := manager.PrepareLocalDelete(t.Context(), localDeleteSelectionForTest(t, filename))
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	run := &remoteRun{ctx: ctx, cancel: cancel}
	registration := make(chan error, 1)
	go func() {
		registration <- manager.protectLocalTransferRun(run, TransferJob{Operation: RemotePut, SourcePath: filename})
	}()
	cancel()
	plan.Close()
	if err := <-registration; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled registration = %v", err)
	}
	if len(manager.localTransferRuns) != 0 {
		t.Fatal("a cancelled worker acquired a local path")
	}
	assertLocalMutationFile(t, filename, "keep")
}

package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	moment := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		moment = moment.Add(time.Second)
		return moment
	}
}

func newTestManager(t *testing.T) (*Manager, *Workspace) {
	t.Helper()
	workspace := newTestWorkspace(t)
	return NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096))), workspace
}

func writeWorkspaceFile(t *testing.T, workspace *Workspace, name, contents string, permission fs.FileMode) string {
	t.Helper()
	path := filepath.Join(workspace.Root(), name)
	if privateStateContains(workspace.StateDir(), path) {
		if err := workspace.EnsureDirectory(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		temporary, err := workspace.FileSystem().WriteTemp(filepath.Dir(path), temporaryPrefix, permission, []byte(contents))
		if err != nil {
			t.Fatal(err)
		}
		if err := workspace.FileSystem().Rename(temporary, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), permission); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommitWritesEveryChangeAndRecordsHistory(t *testing.T) {
	manager, workspace := newTestManager(t)
	config := writeWorkspaceFile(t, workspace, "config", "Host old\n", 0o644)
	extra := filepath.Join(workspace.Root(), "conf.d", "new.conf")
	if err := os.MkdirAll(filepath.Dir(extra), 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{
			{Path: config, Contents: []byte("Host new\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host old\n"))}},
			{Path: extra, Contents: []byte("Host extra\n")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 2 || result.ID == "" {
		t.Fatalf("result = %#v", result)
	}

	for path, want := range map[string]string{config: "Host new\n", extra: "Host extra\n"} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("%s = %q, %v", path, contents, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != FilePermission {
			t.Fatalf("%s permission = %v, want %v", path, info.Mode().Perm(), FilePermission)
		}
	}

	backup, err := os.ReadFile(filepath.Join(result.BackupDir, "config"))
	if err != nil || string(backup) != "Host old\n" {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "conf.d", "new.conf")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("a file that did not exist was backed up")
	}

	journalEntries, err := os.ReadDir(filepath.Join(workspace.StateDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	if len(journalEntries) != 0 {
		t.Fatalf("journal still holds %d entries", len(journalEntries))
	}
	historyEntries, err := os.ReadDir(filepath.Join(workspace.StateDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	if len(historyEntries) != 1 || !strings.HasSuffix(historyEntries[0].Name(), ".json") {
		t.Fatalf("history = %#v", historyEntries)
	}

	staged, err := os.ReadDir(workspace.Root())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range staged {
		if strings.HasPrefix(entry.Name(), ".sshc-") {
			t.Fatalf("temporary file %q was left behind", entry.Name())
		}
	}
}

func TestCommitAppliesAndChecksExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix executable permission bits")
	}
	manager, workspace := newTestManager(t)
	target := writeWorkspaceFile(t, workspace, "scripts/connect", "#!/bin/sh\n", FilePermission)
	digest := Digest([]byte("#!/bin/sh\n"))
	if _, err := manager.Commit(Request{
		Operation: "sync.pull",
		Changes: []Change{{
			Path: target, Contents: []byte("#!/bin/sh\n"), Mode: DirectoryPermission,
			Precondition: Precondition{Exists: true, Digest: digest, Mode: FilePermission},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != DirectoryPermission {
		t.Fatalf("mode = %04o, want 0700", info.Mode().Perm())
	}
	if _, err := manager.Commit(Request{
		Operation: "sync.pull",
		Changes: []Change{{
			Path: target, Contents: []byte("#!/bin/sh\n"), Mode: FilePermission,
			Precondition: Precondition{Exists: true, Digest: digest, Mode: FilePermission},
		}},
	}); err == nil {
		t.Fatal("mode-only concurrent change was not rejected")
	}
}

func TestAfterCommitReportsOnlySuccessfulMutations(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := filepath.Join(workspace.Root(), "config")
	var operations []string
	manager.AfterCommit = func(operation string) {
		operations = append(operations, operation)
	}

	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: path, Contents: []byte("Host saved\n")}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(Request{}); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("invalid Commit = %v, want ErrInvalidOperation", err)
	}
	if len(operations) != 1 || operations[0] != "config.save" {
		t.Fatalf("AfterCommit operations = %v, want [config.save]", operations)
	}
}

func TestCommitRejectsExternalChangesWithThreeWayData(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "config", "Host disk\n", 0o600)

	_, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: path, Contents: []byte("Host ui\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host base\n"))}}},
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want ConflictError", err)
	}
	if conflict.Path != path || string(conflict.Current) != "Host disk\n" {
		t.Fatalf("conflict = %#v", conflict)
	}
	if conflict.Expected == conflict.Actual {
		t.Fatal("conflict does not distinguish the two versions")
	}
	if strings.Contains(conflict.Error(), "Host disk") {
		t.Fatal("conflict error message leaks file contents")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "Host disk\n" {
		t.Fatalf("file changed during a rejected commit: %q", contents)
	}
}

func TestCommitRejectsCreationWhenTheFileAlreadyExists(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "config", "Host disk\n", 0o600)
	_, err := manager.Commit(Request{
		Operation: "config.create",
		Changes:   []Change{{Path: path, Contents: []byte("Host ui\n")}},
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want ConflictError", err)
	}
}

func TestCommitRejectsInvalidRequests(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := filepath.Join(workspace.Root(), "config")

	if _, err := manager.Commit(Request{Operation: "config.save"}); !errors.Is(err, ErrNoChanges) {
		t.Errorf("empty request error = %v", err)
	}
	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: path}, {Path: path}},
	}); !errors.Is(err, ErrDuplicatePath) {
		t.Errorf("duplicate path error = %v", err)
	}
	outside := filepath.Join(filepath.Dir(workspace.Root()), "outside.conf")
	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: outside}},
	}); !errors.Is(err, ErrOutsideWorkspace) {
		t.Errorf("outside path error = %v", err)
	}
}

func TestWritersRejectEmptyOperationBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Manager, string) error
	}{
		{
			name: "Commit",
			call: func(manager *Manager, target string) error {
				_, err := manager.Commit(Request{
					Changes: []Change{{
						Path:         target,
						Contents:     []byte("after\n"),
						Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
					}},
				})
				return err
			},
		},
		{
			name: "CommitAtomic",
			call: func(manager *Manager, target string) error {
				_, err := manager.CommitAtomic(Request{
					Changes: []Change{{
						Path:         target,
						Contents:     []byte("after\n"),
						Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
					}},
				})
				return err
			},
		},
		{
			name: "Note",
			call: func(manager *Manager, target string) error {
				_, err := manager.Note("", []string{target})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, workspace := newTestManager(t)
			target := writeWorkspaceFile(t, workspace, "config", "before\n", 0o600)
			if err := test.call(manager, target); !errors.Is(err, ErrInvalidOperation) {
				t.Fatalf("%s empty operation = %v, want ErrInvalidOperation", test.name, err)
			}
			if contents, err := os.ReadFile(target); err != nil || string(contents) != "before\n" {
				t.Fatalf("target after rejected %s = %q, %v", test.name, contents, err)
			}
			if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("%s mutated state before rejecting operation: %v", test.name, err)
			}
		})
	}
}

func TestNoteRejectsDuplicatePathsBeforeMutation(t *testing.T) {
	manager, workspace := newTestManager(t)
	target := writeWorkspaceFile(t, workspace, "config", "before\n", 0o600)
	if _, err := manager.Note("config.inspect", []string{target, target}); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("Note duplicate paths = %v, want ErrDuplicatePath", err)
	}
	if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Note mutated state before rejecting duplicate: %v", err)
	}
}

func TestCommitLeavesRecoverableJournalWhenRenameFails(t *testing.T) {
	workspace := newTestWorkspace(t)
	first := writeWorkspaceFile(t, workspace, "first.conf", "Host first\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "second.conf", "Host second\n", 0o600)
	failure := errors.New("injected rename failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == second {
				return failure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))

	_, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{
			{Path: first, Contents: []byte("Host first changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host first\n"))}},
			{Path: second, Contents: []byte("Host second changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host second\n"))}},
		},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the injected failure", err)
	}

	if contents, readErr := os.ReadFile(first); readErr != nil || string(contents) != "Host first changed\n" {
		t.Fatalf("first file = %q, %v", contents, readErr)
	}
	if contents, readErr := os.ReadFile(second); readErr != nil || string(contents) != "Host second\n" {
		t.Fatalf("second file = %q, %v", contents, readErr)
	}
	journalEntries, readErr := os.ReadDir(filepath.Join(workspace.StateDir(), "journal"))
	if readErr != nil || len(journalEntries) != 1 {
		t.Fatalf("journal = %#v, %v", journalEntries, readErr)
	}
}

func TestCommitRunsTheInjectedValidatorBeforeTouchingDisk(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "config", "Host old\n", 0o600)
	rejected := errors.New("syntax error at line 1")
	manager.Validate = func(request Request) error {
		if len(request.Changes) != 1 || request.Operation != "config.save" {
			t.Fatalf("validator received %#v", request)
		}
		return rejected
	}

	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: path, Contents: []byte("Host new\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host old\n"))}}},
	}); !errors.Is(err, rejected) {
		t.Fatalf("error = %v, want the validator's error", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "Host old\n" {
		t.Fatalf("file changed despite validation failure: %q", contents)
	}
	if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("a rejected request created state directories")
	}
}

func TestCommitFailureWhileStagingLeavesEveryFileUntouched(t *testing.T) {
	workspace := newTestWorkspace(t)
	first := writeWorkspaceFile(t, workspace, "first.conf", "Host first\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "conf.d/second.conf", "Host second\n", 0o600)
	failure := errors.New("injected staging failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "writeTemp" && path == filepath.Dir(second) {
				return failure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))

	_, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{
			{Path: first, Contents: []byte("Host first changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host first\n"))}},
			{Path: second, Contents: []byte("Host second changed\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host second\n"))}},
		},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the injected failure", err)
	}
	for path, want := range map[string]string{first: "Host first\n", second: "Host second\n"} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil || string(contents) != want {
			t.Fatalf("%s = %q, %v", path, contents, readErr)
		}
	}
	journalEntries, readErr := os.ReadDir(filepath.Join(workspace.StateDir(), "journal"))
	if readErr != nil || len(journalEntries) != 1 {
		t.Fatalf("journal = %#v, %v", journalEntries, readErr)
	}
	// 何も rename されなかったので、Task 7 の復旧テストは、ファイルをひとつも復元
	// せずにこれを巻き戻せる。
}

func TestCommitAtomicRollsBackAppliedWritesForLateFailures(t *testing.T) {
	tests := []struct {
		name   string
		failOn func(root, first, second string) func(string, string) error
	}{
		{
			name: "second target rename",
			failOn: func(_, _, second string) func(string, string) error {
				failed := false
				return func(operation, path string) error {
					if !failed && operation == "rename" && path == second {
						failed = true
						return errors.New("second rename failed")
					}
					return nil
				}
			},
		},
		{
			name: "target directory sync",
			failOn: func(root, _, _ string) func(string, string) error {
				failed := false
				return func(operation, path string) error {
					if !failed && operation == "syncDir" && path == root {
						failed = true
						return errors.New("target sync failed")
					}
					return nil
				}
			},
		},
		{
			name: "journal progress update",
			failOn: func(root, _, _ string) func(string, string) error {
				journalRenames := 0
				journalDirectory := filepath.Join(root, "sshc", journalDirectoryName)
				return func(operation, path string) error {
					if operation == "rename" && filepath.Dir(path) == journalDirectory {
						journalRenames++
						if journalRenames == 3 {
							return errors.New("journal progress update failed")
						}
					}
					return nil
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := newTestWorkspace(t)
			first := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
			second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
			workspace.fileSystem = faultyFileSystem{
				FileSystem: OSFileSystem{},
				failOn:     test.failOn(workspace.Root(), first, second),
			}
			manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x41}, 4096)))
			_, err := manager.CommitAtomic(Request{
				Operation: "connection.update",
				Changes: []Change{
					{Path: first, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
					{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
				},
			})
			if err == nil {
				t.Fatal("CommitAtomic unexpectedly succeeded")
			}
			for path, want := range map[string]string{first: "first before\n", second: "second before\n"} {
				contents, readErr := os.ReadFile(path)
				if readErr != nil || string(contents) != want {
					t.Fatalf("%s = %q, %v; want %q", path, contents, readErr, want)
				}
			}
			pending, pendingErr := manager.Pending()
			if pendingErr != nil {
				t.Fatal(pendingErr)
			}
			if len(pending) != 0 {
				t.Fatalf("pending after atomic rollback = %#v", pending)
			}
		})
	}
}

func TestCommitAtomicRecoveryReconstructsProgressAfterRollbackAlsoFails(t *testing.T) {
	workspace := newTestWorkspace(t)
	first := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
	journalRenames := 0
	firstTargetRenames := 0
	failure := errors.New("injected nested recovery failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation != "rename" {
				return nil
			}
			if filepath.Dir(path) == filepath.Join(workspace.Root(), "sshc", journalDirectoryName) {
				journalRenames++
				// 3 回目の journal 置換は最初に適用した対象を記録し、4 回目は
				// CommitAtomic がロールバック前に進捗を永続化する処理である。
				if journalRenames == 3 || journalRenames == 4 {
					return failure
				}
			}
			if path == first {
				firstTargetRenames++
				if firstTargetRenames == 2 {
					return failure
				}
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096)))
	result, err := manager.CommitAtomic(Request{
		Operation: "connection.update",
		Changes: []Change{
			{Path: first, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
			{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
		},
	})
	if !errors.Is(err, failure) || result.ID == "" {
		t.Fatalf("CommitAtomic = %#v, %v", result, err)
	}
	if got, readErr := os.ReadFile(first); readErr != nil || string(got) != "first after\n" {
		t.Fatalf("first after failed rollback = %q, %v", got, readErr)
	}
	journalPath := filepath.Join(workspace.Root(), "sshc", journalDirectoryName, result.ID+".json")
	journalBytes, readErr := os.ReadFile(journalPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var stale journalRecord
	if err := json.Unmarshal(journalBytes, &stale); err != nil {
		t.Fatal(err)
	}
	if stale.Committed != 0 {
		t.Fatalf("fixture journal committed = %d, want stale 0", stale.Committed)
	}

	if err := manager.Rollback(result.ID); err != nil {
		t.Fatalf("later Rollback = %v", err)
	}
	for path, want := range map[string]string{first: "first before\n", second: "second before\n"} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil || string(contents) != want {
			t.Fatalf("%s = %q, %v; want %q", path, contents, readErr, want)
		}
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after later rollback = %#v, %v", pending, err)
	}
}

func TestAtomicRenameBeforeSyncCrashStateReconcilesAndRollsBackAfterRestart(t *testing.T) {
	workspace := newTestWorkspace(t)
	target := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	syncFailed := false
	targetRenames := 0
	syncFailure := errors.New("target sync failed after rename")
	rollbackFailure := errors.New("immediate rollback failed")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == target {
				targetRenames++
				if targetRenames == 2 {
					return rollbackFailure
				}
			}
			if operation == "syncDir" && path == workspace.Root() && !syncFailed {
				syncFailed = true
				return syncFailure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x6b}, 4096)))
	result, err := manager.CommitAtomic(Request{
		Operation: "connection.update",
		Changes: []Change{{
			Path:         target,
			Contents:     []byte("Host after\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))},
		}},
	})
	if !errors.Is(err, syncFailure) || !errors.Is(err, rollbackFailure) || result.ID == "" {
		t.Fatalf("CommitAtomic = %#v, %v; want sync and rollback failures", result, err)
	}
	journalPath := filepath.Join(workspace.StateDir(), journalDirectoryName, result.ID+".json")
	journalBody, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var emitted journalRecord
	if err := json.Unmarshal(journalBody, &emitted); err != nil {
		t.Fatal(err)
	}
	// rename がステージ済みファイルを消費した時点で、記録はそれを手放している。
	// 進捗の数え上げより先にそうすることが、失敗経路の残す記録を、読み手が受理する
	// 形に保っている。
	if emitted.Status != statusStaged || emitted.Committed != 1 || emitted.Entries[0].Temp != "" {
		t.Fatalf("emitted crash record = %#v, want staged committed write without its consumed temp", emitted)
	}
	if contents, readErr := os.ReadFile(target); readErr != nil || string(contents) != "Host after\n" {
		t.Fatalf("target after failed immediate rollback = %q, %v", contents, readErr)
	}

	workspace.fileSystem = OSFileSystem{}
	restarted := NewManager(workspace, func() time.Time {
		return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	}, bytes.NewReader(bytes.Repeat([]byte{0x6c}, 4096)))
	pending, err := restarted.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending after restart = %#v, %v", pending, err)
	}
	if pending[0].Committed != 1 || pending[0].CanComplete || !pending[0].CanRollback {
		t.Fatalf("reconciled pending = %#v", pending[0])
	}
	loaded, _, err := restarted.loadPending(result.ID)
	if err != nil {
		t.Fatalf("loadPending after reconcile = %v", err)
	}
	if loaded.Entries[0].Temp != "" {
		t.Fatalf("reconciled committed write retained temp %q", loaded.Entries[0].Temp)
	}
	if err := restarted.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback after restart = %v", err)
	}
	if contents, readErr := os.ReadFile(target); readErr != nil || string(contents) != "Host before\n" {
		t.Fatalf("target after recovery rollback = %q, %v", contents, readErr)
	}
	history, err := restarted.History()
	if err != nil || len(history) != 1 || history[0].Status != statusRolledBack {
		t.Fatalf("History after recovery rollback = %#v, %v", history, err)
	}
}

func TestAtomicRollbackRenameBeforeSyncCrashStateReconcilesWithoutStagedTemp(t *testing.T) {
	workspace := newTestWorkspace(t)
	target := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	historyFailureInjected := false
	rollbackSyncFailureInjected := false
	historyFailure := errors.New("history publication failed")
	rollbackSyncFailure := errors.New("rollback target sync failed after rename")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && filepath.Dir(path) == filepath.Join(workspace.StateDir(), historyDirectoryName) && !historyFailureInjected {
				historyFailureInjected = true
				return historyFailure
			}
			if operation == "syncDir" && path == workspace.Root() && historyFailureInjected && !rollbackSyncFailureInjected {
				rollbackSyncFailureInjected = true
				return rollbackSyncFailure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x6d}, 4096)))
	result, err := manager.CommitAtomic(Request{
		Operation: "connection.update",
		Changes: []Change{{
			Path:         target,
			Contents:     []byte("Host after\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))},
		}},
	})
	if !errors.Is(err, historyFailure) || !errors.Is(err, rollbackSyncFailure) || result.ID == "" {
		t.Fatalf("CommitAtomic = %#v, %v; want history and rollback sync failures", result, err)
	}
	if contents, readErr := os.ReadFile(target); readErr != nil || string(contents) != "Host before\n" {
		t.Fatalf("target after rollback rename = %q, %v", contents, readErr)
	}
	journalPath := filepath.Join(workspace.StateDir(), journalDirectoryName, result.ID+".json")
	journalBody, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var emitted journalRecord
	if err := json.Unmarshal(journalBody, &emitted); err != nil {
		t.Fatal(err)
	}
	if emitted.Status != statusStaged || emitted.Committed != 1 || emitted.Entries[0].Temp != "" {
		t.Fatalf("emitted rollback crash record = %#v, want staged committed write without temp", emitted)
	}

	workspace.fileSystem = OSFileSystem{}
	restarted := NewManager(workspace, func() time.Time {
		return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	}, bytes.NewReader(bytes.Repeat([]byte{0x6e}, 4096)))
	pending, err := restarted.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending after rollback sync crash = %#v, %v", pending, err)
	}
	if pending[0].Committed != 0 || pending[0].CanComplete || !pending[0].CanRollback {
		t.Fatalf("reconciled rollback progress = %#v", pending[0])
	}
	if err := restarted.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback after restart = %v", err)
	}
	history, err := restarted.History()
	if err != nil || len(history) != 1 || history[0].Status != statusRolledBack {
		t.Fatalf("History after reconciled rollback = %#v, %v", history, err)
	}
}

// commitWithStaleJournal は、最初のエントリの適用には成功し、それを記録しようと
// するジャーナル書き込みがすべて失敗するコミットを走らせる。残る永続記録は、
// ファイルシステムが実際に保持しているより少ない進捗を名乗ることになる。
func commitWithStaleJournal(t *testing.T, workspace *Workspace, filler byte, request Request) string {
	t.Helper()
	journalDirectory := filepath.Join(workspace.StateDir(), journalDirectoryName)
	journalRenames := 0
	failure := errors.New("injected journal progress failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			// 3 番目は最初のエントリを適用したあとの進捗更新、4 番目は失敗経路が
			// その進捗を残そうとする再試行である。両方を落とすことでのみ、復旧は
			// 対象の状態から数え直すほかなくなる。
			if operation == "rename" && filepath.Dir(path) == journalDirectory {
				journalRenames++
				if journalRenames == 3 || journalRenames == 4 {
					return failure
				}
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{filler}, 4096)))
	result, err := manager.Commit(request)
	if !errors.Is(err, failure) || result.ID == "" {
		t.Fatalf("Commit = %#v, %v; want the injected journal failure", result, err)
	}
	body, readErr := os.ReadFile(filepath.Join(journalDirectory, result.ID+".json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var emitted journalRecord
	if err := json.Unmarshal(body, &emitted); err != nil {
		t.Fatal(err)
	}
	if emitted.Status != statusStaged || emitted.Committed != 0 {
		t.Fatalf("durable record = %#v, want staged progress stale at 0", emitted)
	}
	workspace.fileSystem = OSFileSystem{}
	return result.ID
}

func restartedManager(t *testing.T, workspace *Workspace) *Manager {
	t.Helper()
	return NewManager(workspace, func() time.Time {
		return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	}, bytes.NewReader(bytes.Repeat([]byte{0x72}, 4096)))
}

func reconciledPending(t *testing.T, manager *Manager, id string, committed int) Pending {
	t.Helper()
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("Pending = %#v, %v", pending, err)
	}
	if pending[0].Committed != committed {
		t.Fatalf("reconciled progress = %d, want %d", pending[0].Committed, committed)
	}
	return pending[0]
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != want {
		t.Fatalf("%s = %q, %v; want %q", filepath.Base(path), contents, err, want)
	}
}

func stagedFileNames(t *testing.T, workspace *Workspace) []string {
	t.Helper()
	entries, err := os.ReadDir(workspace.Root())
	if err != nil {
		t.Fatal(err)
	}
	var staged []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), temporaryPrefix) {
			staged = append(staged, entry.Name())
		}
	}
	return staged
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s still exists: %v", filepath.Base(path), err)
	}
}

// TestRecoveryReconstructsNonAtomicProgressFromAStaleJournal は、遅れた進捗数を
// そのまま信じた復旧が、実際には行っていない巻き戻しを成功として報告することを
// 防ぐ。バックアップを意図して残さなかった削除では、その偽の成功が、取り返しの
// つかない消失を覆い隠すことになる。
func TestRecoveryReconstructsNonAtomicProgressFromAStaleJournal(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		workspace := newTestWorkspace(t)
		first := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
		second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
		id := commitWithStaleJournal(t, workspace, 0x71, Request{
			Operation: "connection.update",
			Changes: []Change{
				{Path: first, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
				{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
			},
		})
		assertFileContents(t, first, "first after\n")

		manager := restartedManager(t, workspace)
		if item := reconciledPending(t, manager, id, 1); !item.CanRollback {
			t.Fatalf("reconciled applied write = %#v, want a rollback offer", item)
		}
		if err := manager.Rollback(id); err != nil {
			t.Fatalf("Rollback = %v", err)
		}
		assertFileContents(t, first, "first before\n")
		assertFileContents(t, second, "second before\n")
	})

	t.Run("move", func(t *testing.T) {
		workspace := newTestWorkspace(t)
		first := writeWorkspaceFile(t, workspace, "first.conf", "first bytes\n", 0o600)
		second := writeWorkspaceFile(t, workspace, "second.conf", "second bytes\n", 0o600)
		firstTarget := filepath.Join(workspace.Root(), "first.moved.conf")
		secondTarget := filepath.Join(workspace.Root(), "second.moved.conf")
		id := commitWithStaleJournal(t, workspace, 0x75, Request{
			Operation: "key.relocate",
			Moves: []Move{
				{From: first, To: firstTarget, Precondition: Precondition{Exists: true, Digest: Digest([]byte("first bytes\n"))}},
				{From: second, To: secondTarget, Precondition: Precondition{Exists: true, Digest: Digest([]byte("second bytes\n"))}},
			},
		})
		assertFileContents(t, firstTarget, "first bytes\n")
		assertMissing(t, first)

		manager := restartedManager(t, workspace)
		reconciledPending(t, manager, id, 1)
		if err := manager.Rollback(id); err != nil {
			t.Fatalf("Rollback = %v", err)
		}
		assertFileContents(t, first, "first bytes\n")
		assertMissing(t, firstTarget)
		assertFileContents(t, second, "second bytes\n")
		assertMissing(t, secondTarget)
	})

	t.Run("removal with backup", func(t *testing.T) {
		workspace := newTestWorkspace(t)
		first := writeWorkspaceFile(t, workspace, "first.conf", "first bytes\n", 0o600)
		second := writeWorkspaceFile(t, workspace, "second.conf", "second bytes\n", 0o600)
		id := commitWithStaleJournal(t, workspace, 0x76, Request{
			Operation: "connection.delete",
			Removals: []Removal{
				{Path: first, Precondition: Precondition{Exists: true, Digest: Digest([]byte("first bytes\n"))}, Backup: true},
				{Path: second, Precondition: Precondition{Exists: true, Digest: Digest([]byte("second bytes\n"))}, Backup: true},
			},
		})
		assertMissing(t, first)

		manager := restartedManager(t, workspace)
		reconciledPending(t, manager, id, 1)
		if err := manager.Rollback(id); err != nil {
			t.Fatalf("Rollback = %v", err)
		}
		assertFileContents(t, first, "first bytes\n")
		assertFileContents(t, second, "second bytes\n")
	})

	t.Run("removal without backup", func(t *testing.T) {
		workspace := newTestWorkspace(t)
		first := writeWorkspaceFile(t, workspace, "id_first", "PRIVATE KEY ONE\n", 0o600)
		second := writeWorkspaceFile(t, workspace, "id_second", "PRIVATE KEY TWO\n", 0o600)
		id := commitWithStaleJournal(t, workspace, 0x77, Request{
			Operation: "key.delete",
			Removals: []Removal{
				{Path: first, Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY ONE\n"))}},
				{Path: second, Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY TWO\n"))}},
			},
		})
		assertMissing(t, first)

		manager := restartedManager(t, workspace)
		if item := reconciledPending(t, manager, id, 1); item.CanRollback {
			t.Fatal("an applied removal that kept no backup was offered as reversible")
		}
		if err := manager.Rollback(id); !errors.Is(err, ErrIrreversibleRemoval) {
			t.Fatalf("Rollback = %v, want ErrIrreversibleRemoval", err)
		}
		assertMissing(t, first)
		assertFileContents(t, second, "PRIVATE KEY TWO\n")
		history, err := manager.History()
		if err != nil || len(history) != 0 {
			t.Fatalf("History after refused rollback = %#v, %v", history, err)
		}
	})
}

// TestRollbackRetryResumesFromReconciledProgressAfterAPartialFailure は、
// 途中まで戻して失敗した巻き戻しの再試行を扱う。移動の復元は冪等ではないので、
// 減っていない進捗数から再開すると、すでに戻したエントリで永久に立ち往生する。
func TestRollbackRetryResumesFromReconciledProgressAfterAPartialFailure(t *testing.T) {
	workspace := newTestWorkspace(t)
	names := []string{"one", "two", "three", "four"}
	sources := make([]string, 0, len(names))
	targets := make([]string, 0, len(names))
	moves := make([]Move, 0, len(names))
	for _, name := range names {
		contents := "Host " + name + "\n"
		source := writeWorkspaceFile(t, workspace, name+".conf", contents, 0o600)
		target := filepath.Join(workspace.Root(), name+".moved.conf")
		sources = append(sources, source)
		targets = append(targets, target)
		moves = append(moves, Move{From: source, To: target, Precondition: Precondition{Exists: true, Digest: Digest([]byte(contents))}})
	}

	commitFailure := errors.New("injected fourth move failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == targets[3] {
				return commitFailure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x73}, 4096)))
	result, err := manager.Commit(Request{Operation: "key.relocate", Moves: moves})
	if !errors.Is(err, commitFailure) || result.ID == "" {
		t.Fatalf("Commit = %#v, %v; want the injected move failure", result, err)
	}

	rollbackFailure := errors.New("injected move restore failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == sources[1] {
				return rollbackFailure
			}
			return nil
		},
	}
	if err := manager.Rollback(result.ID); !errors.Is(err, rollbackFailure) {
		t.Fatalf("Rollback = %v, want the injected restore failure", err)
	}
	// 巻き戻しは上から進む。三つ目だけが戻り、その進捗はどこにも記録されていない。
	assertFileContents(t, sources[2], "Host three\n")
	assertMissing(t, targets[2])
	assertFileContents(t, targets[1], "Host two\n")

	workspace.fileSystem = OSFileSystem{}
	restarted := restartedManager(t, workspace)
	reconciledPending(t, restarted, result.ID, 2)
	if err := restarted.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback retry = %v", err)
	}
	for index, source := range sources {
		assertFileContents(t, source, "Host "+names[index]+"\n")
		assertMissing(t, targets[index])
	}
	history, err := restarted.History()
	if err != nil || len(history) != 1 || history[0].Status != statusRolledBack {
		t.Fatalf("History after retried rollback = %#v, %v", history, err)
	}
}

// 記録の中には、対象を見ても何も分からないエントリがある。書いても内容の変わらない
// 置き換えと、もとからあったディレクトリの作成である。これを「適用済み」と読むと、
// ひとつ前が未適用のときに、ありもしない矛盾ができあがる。
func TestReconcileCountsEvidenceFreeEntriesInsideTheAppliedPrefix(t *testing.T) {
	manager, workspace := newTestManager(t)
	unapplied := writeWorkspaceFile(t, workspace, "first.conf", "before\n", 0o600)
	unappliedEntry := journalEntry{
		Action:         actionWrite,
		Path:           unapplied,
		Temp:           filepath.Join(workspace.Root(), temporaryPrefix+validJournalTestID+"-staged"),
		HadPrevious:    true,
		Mode:           0o600,
		Digest:         Digest([]byte("after\n")),
		PreviousDigest: Digest([]byte("before\n")),
	}

	existingDirectory := filepath.Join(workspace.Root(), "conf.d")
	if err := workspace.EnsureDirectory(existingDirectory); err != nil {
		t.Fatal(err)
	}
	unchanged := writeWorkspaceFile(t, workspace, "meta.json", "{}\n", 0o600)

	for name, evidenceFree := range map[string]journalEntry{
		"directory that already existed": {
			Action:      actionMakeDir,
			Path:        existingDirectory,
			HadPrevious: true,
			Mode:        uint32(DirectoryPermission),
		},
		"write whose contents do not change": {
			Action:         actionWrite,
			Path:           unchanged,
			HadPrevious:    true,
			Mode:           0o600,
			Digest:         Digest([]byte("{}\n")),
			PreviousDigest: Digest([]byte("{}\n")),
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := journalRecord{
				ID:        validJournalTestID,
				Version:   journalVersion,
				Operation: "config.move",
				Status:    statusStaged,
				StartedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
				Entries:   []journalEntry{unappliedEntry, evidenceFree},
			}
			if _, err := manager.reconcileRecord(&record); err != nil {
				t.Fatalf("reconcileRecord = %v, want no error", err)
			}
			if record.Committed != 0 {
				t.Fatalf("reconciled progress = %d, want 0", record.Committed)
			}
		})
	}
}

// application 層は metadata の書き込みを毎回、変わっていなくても最後に足す。
// つまりこの形は例外ではなく、日常のトランザクションである。
func TestRecoveryHandlesAnUnchangedTrailingWriteAfterNothingWasApplied(t *testing.T) {
	workspace := newTestWorkspace(t)
	changed := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	unchanged := writeWorkspaceFile(t, workspace, "meta.json", "{}\n", 0o600)
	failure := errors.New("injected first rename failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == changed {
				return failure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x78}, 4096)))
	result, err := manager.Commit(Request{
		Operation: "config.move",
		Changes: []Change{
			{Path: changed, Contents: []byte("Host after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))}},
			{Path: unchanged, Contents: []byte("{}\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("{}\n"))}},
		},
	})
	if !errors.Is(err, failure) || result.ID == "" {
		t.Fatalf("Commit = %#v, %v; want the injected rename failure", result, err)
	}

	workspace.fileSystem = OSFileSystem{}
	restarted := restartedManager(t, workspace)
	item := reconciledPending(t, restarted, result.ID, 0)
	if !item.CanRollback {
		t.Fatalf("nothing was applied, yet the transaction was not reversible: %#v", item)
	}
	if err := restarted.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback = %v", err)
	}
	assertFileContents(t, changed, "Host before\n")
	assertFileContents(t, unchanged, "{}\n")
}

// 一覧は何も書き換えない。走っている最中のトランザクションの記録も同じ経路で
// 読まれるので、ここで一時ファイルを消すと、進行中の保存が消えたファイルを
// rename しようとして失敗する。片付けるのは、終わらせるときだけである。
func TestRecoveryReleasesAnEvidenceFreeStagedFileOnlyWhenTheTransactionEnds(t *testing.T) {
	workspace := newTestWorkspace(t)
	changed := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	unchanged := writeWorkspaceFile(t, workspace, "meta.json", "{}\n", 0o600)
	id := commitWithStaleJournal(t, workspace, 0x79, Request{
		Operation: "config.move",
		Changes: []Change{
			{Path: changed, Contents: []byte("Host after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))}},
			{Path: unchanged, Contents: []byte("{}\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("{}\n"))}},
		},
	})
	assertFileContents(t, changed, "Host after\n")

	staged := stagedFileNames(t, workspace)
	if len(staged) == 0 {
		t.Fatal("the fixture produced no staged file to observe")
	}

	restarted := restartedManager(t, workspace)
	reconciledPending(t, restarted, id, 2)

	if after := stagedFileNames(t, workspace); len(after) != len(staged) {
		t.Fatalf("listing the pending transactions changed the staged files: %v then %v", staged, after)
	}

	if err := restarted.Rollback(id); err != nil {
		t.Fatalf("Rollback = %v", err)
	}
	if after := stagedFileNames(t, workspace); len(after) != 0 {
		t.Fatalf("finishing the transaction left the staged files %v behind", after)
	}
	assertFileContents(t, changed, "Host before\n")
	assertFileContents(t, unchanged, "{}\n")
}

// 判別できない記録がひとつあることは、他の記録も履歴も見えなくなる理由にならない。
// 呼び出し側はこの一覧で設定画面全体を組み立てている。
func TestPendingReportsAnUnreadableRecordWithoutFailingTheWholeListing(t *testing.T) {
	workspace := newTestWorkspace(t)
	tampered := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
	tamperedID := commitWithStaleJournal(t, workspace, 0x7a, Request{
		Operation: "connection.update",
		Changes: []Change{
			{Path: tampered, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
			{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
		},
	})

	third := writeWorkspaceFile(t, workspace, "third.conf", "third before\n", 0o600)
	fourth := writeWorkspaceFile(t, workspace, "fourth.conf", "fourth before\n", 0o600)
	readableID := commitWithStaleJournal(t, workspace, 0x7b, Request{
		Operation: "connection.update",
		Changes: []Change{
			{Path: third, Contents: []byte("third after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("third before\n"))}},
			{Path: fourth, Contents: []byte("fourth after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("fourth before\n"))}},
		},
	})

	// 中断されたトランザクションが触れるはずだったファイルを、外から書き換える。
	// これで、記録のどちらの状態とも一致しなくなる。
	if err := os.WriteFile(tampered, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := restartedManager(t, workspace)
	pending, err := restarted.Pending()
	if err != nil {
		t.Fatalf("Pending = %v, want the listing to survive one unreadable record", err)
	}
	if len(pending) != 2 {
		t.Fatalf("Pending = %#v, want both transactions", pending)
	}
	for _, item := range pending {
		switch item.ID {
		case tamperedID:
			if item.CanComplete || item.CanRollback {
				t.Fatalf("an unreadable transaction was offered as actionable: %#v", item)
			}
		case readableID:
			if !item.CanRollback {
				t.Fatalf("the readable transaction lost its rollback offer: %#v", item)
			}
		default:
			t.Fatalf("unexpected pending transaction %q", item.ID)
		}
	}

	if err := restarted.Rollback(tamperedID); !errors.Is(err, ErrRecoveryStateUnknown) {
		t.Fatalf("Rollback(unreadable) = %v, want ErrRecoveryStateUnknown", err)
	}
	if err := restarted.Complete(tamperedID); !errors.Is(err, ErrRecoveryStateUnknown) {
		t.Fatalf("Complete(unreadable) = %v, want ErrRecoveryStateUnknown", err)
	}
	assertFileContents(t, tampered, "edited by hand\n")

	if err := restarted.Rollback(readableID); err != nil {
		t.Fatalf("Rollback(readable) = %v", err)
	}
	assertFileContents(t, third, "third before\n")
}

// 一覧は、走っている最中のトランザクションの記録も同じ経路で読む。そこで
// ファイルに触れれば、保存の途中で足元のステージ済みファイルが消える。
func TestListingPendingTransactionsDoesNotDisturbACommitInFlight(t *testing.T) {
	workspace := newTestWorkspace(t)
	changed := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	unchanged := writeWorkspaceFile(t, workspace, "meta.json", "{}\n", 0o600)

	var observer *Manager
	listings := 0
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			// 最初の対象を書き換えたあと、進捗を記録し直す前。一覧が走っている
			// 記録を読むのは、ちょうどこの隙間である。
			if operation == "syncDir" && path == workspace.Root() && listings == 0 {
				listings++
				if _, err := observer.Pending(); err != nil {
					t.Errorf("Pending during a commit = %v", err)
				}
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x7c}, 4096)))
	observer = NewManager(workspace, func() time.Time {
		return time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	}, bytes.NewReader(bytes.Repeat([]byte{0x7d}, 4096)))

	if _, err := manager.Commit(Request{
		Operation: "config.move",
		Changes: []Change{
			{Path: changed, Contents: []byte("Host after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))}},
			{Path: unchanged, Contents: []byte("{}\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("{}\n"))}},
		},
	}); err != nil {
		t.Fatalf("Commit with a concurrent listing = %v", err)
	}
	if listings != 1 {
		t.Fatalf("the listing did not run inside the commit (%d)", listings)
	}
	assertFileContents(t, changed, "Host after\n")
	assertFileContents(t, unchanged, "{}\n")
	if staged := stagedFileNames(t, workspace); len(staged) != 0 {
		t.Fatalf("staged files left behind: %v", staged)
	}
	pending, err := observer.Pending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("Pending after the commit = %#v, %v", pending, err)
	}
}

// 書いても中身が変わらない置き換えは、控えを残さなかったとしても巻き戻せる。
// 戻したあとの対象は同じバイト列であり、失うものが無いからである。
func TestAnUnchangedWriteThatKeptNoBackupStaysReversible(t *testing.T) {
	workspace := newTestWorkspace(t)
	key := writeWorkspaceFile(t, workspace, "id_work", "PRIVATE KEY BYTES\n", 0o600)
	config := writeWorkspaceFile(t, workspace, "config", "Host before\n", 0o600)
	failure := errors.New("injected second rename failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "rename" && path == config {
				return failure
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x7e}, 4096)))
	result, err := manager.Commit(Request{
		Operation: "key.reseal",
		Changes: []Change{
			{
				Path:         key,
				Contents:     []byte("PRIVATE KEY BYTES\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY BYTES\n"))},
				SkipBackup:   true,
			},
			{Path: config, Contents: []byte("Host after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host before\n"))}},
		},
	})
	if !errors.Is(err, failure) || result.ID == "" {
		t.Fatalf("Commit = %#v, %v; want the injected rename failure", result, err)
	}

	workspace.fileSystem = OSFileSystem{}
	restarted := restartedManager(t, workspace)
	item := reconciledPending(t, restarted, result.ID, 1)
	if !item.CanRollback {
		t.Fatalf("a write that changed nothing was reported irreversible: %#v", item)
	}
	if err := restarted.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback = %v", err)
	}
	assertFileContents(t, key, "PRIVATE KEY BYTES\n")
	assertFileContents(t, config, "Host before\n")
	if staged := stagedFileNames(t, workspace); len(staged) != 0 {
		t.Fatalf("staged files left behind: %v", staged)
	}
	history, err := restarted.History()
	if err != nil || len(history) != 1 || history[0].Status != statusRolledBack {
		t.Fatalf("History = %#v, %v", history, err)
	}
}

func TestAtomicPendingTransactionCanOnlyBeRolledBack(t *testing.T) {
	workspace := newTestWorkspace(t)
	first := writeWorkspaceFile(t, workspace, "first.conf", "first before\n", 0o600)
	second := writeWorkspaceFile(t, workspace, "second.conf", "second before\n", 0o600)
	historyFailureInjected := false
	secondTargetRenames := 0
	failure := errors.New("injected completion and rollback failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation != "rename" {
				return nil
			}
			if !historyFailureInjected && filepath.Dir(path) == filepath.Join(workspace.Root(), "sshc", historyDirectoryName) {
				historyFailureInjected = true
				return failure
			}
			if path == second {
				secondTargetRenames++
				if secondTargetRenames == 2 {
					return failure
				}
			}
			return nil
		},
	}
	manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x43}, 4096)))
	result, err := manager.CommitAtomic(Request{
		Operation: "connection.update",
		Changes: []Change{
			{Path: first, Contents: []byte("first after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("first before\n"))}},
			{Path: second, Contents: []byte("second after\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("second before\n"))}},
		},
	})
	if !errors.Is(err, failure) || result.ID == "" {
		t.Fatalf("CommitAtomic = %#v, %v", result, err)
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending = %#v, %v", pending, err)
	}
	if pending[0].CanComplete {
		t.Fatal("atomic pending transaction was offered as completable")
	}
	if err := manager.Complete(result.ID); !errors.Is(err, ErrCannotComplete) {
		t.Fatalf("Complete(atomic) = %v, want ErrCannotComplete", err)
	}
	for path, want := range map[string]string{first: "first after\n", second: "second after\n"} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil || string(contents) != want {
			t.Fatalf("rejected Complete changed %s to %q, %v", path, contents, readErr)
		}
	}
	if err := manager.Rollback(result.ID); err != nil {
		t.Fatalf("Rollback(atomic) = %v", err)
	}
}

// faultyFileSystem は、それ以外は本物のファイルシステムに失敗をひとつ注入する。
// これによりテストは、選んだ段階でトランザクションを中断できる。
type faultyFileSystem struct {
	FileSystem
	failOn func(operation, path string) error
}

func (f faultyFileSystem) Rename(oldPath, newPath string) error {
	if err := f.failOn("rename", newPath); err != nil {
		return err
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f faultyFileSystem) MovePrivate(oldPath, newPath string) error {
	if err := f.failOn("rename", newPath); err != nil {
		return err
	}
	return f.FileSystem.MovePrivate(oldPath, newPath)
}

func (f faultyFileSystem) WriteTemp(directory, prefix string, permission fs.FileMode, contents []byte) (string, error) {
	if err := f.failOn("writeTemp", directory); err != nil {
		return "", err
	}
	return f.FileSystem.WriteTemp(directory, prefix, permission, contents)
}

func (f faultyFileSystem) SyncDir(path string) error {
	if err := f.failOn("syncDir", path); err != nil {
		return err
	}
	return f.FileSystem.SyncDir(path)
}

func (f faultyFileSystem) Remove(path string) error {
	if err := f.failOn("remove", path); err != nil {
		return err
	}
	return f.FileSystem.Remove(path)
}

func TestCommitMovesAFileWithoutCopyingItsBytes(t *testing.T) {
	manager, workspace := newTestManager(t)
	source := writeWorkspaceFile(t, workspace, "id_work", "PRIVATE KEY BYTES\n", 0o400)
	destinationDirectory := filepath.Join(workspace.StateDir(), "trash", "entry-1")
	if err := workspace.EnsureDirectory(destinationDirectory); err != nil {
		t.Fatalf("EnsureDirectory error = %v", err)
	}
	destination := filepath.Join(destinationDirectory, "id_work")

	result, err := manager.Commit(Request{
		Operation: "key.trash",
		Moves: []Move{{
			From:         source,
			To:           destination,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY BYTES\n"))},
		}},
	})
	if err != nil {
		t.Fatalf("Commit error = %v", err)
	}

	if _, statErr := os.Lstat(source); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("source still exists: %v", statErr)
	}
	moved, err := os.Lstat(destination)
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if runtime.GOOS != "windows" && moved.Mode().Perm() != 0o400 {
		t.Errorf("destination permission = %04o, want 0400", moved.Mode().Perm())
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "PRIVATE KEY BYTES\n" {
		t.Fatalf("destination contents = %q, %v", contents, err)
	}

	if entries, readErr := os.ReadDir(result.BackupDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("a move copied bytes into the backup directory: %#v", entries)
	}

	history, err := manager.History()
	if err != nil {
		t.Fatalf("History error = %v", err)
	}
	if len(history) != 1 || history[0].Operation != "key.trash" {
		t.Fatalf("history = %#v", history)
	}
}

func TestCommitRejectsAMoveOntoAnExistingFileOrAChangedSource(t *testing.T) {
	manager, workspace := newTestManager(t)
	source := writeWorkspaceFile(t, workspace, "id_work", "ORIGINAL\n", 0o600)
	occupied := writeWorkspaceFile(t, workspace, "taken", "ALREADY HERE\n", 0o600)

	if _, err := manager.Commit(Request{
		Operation: "key.trash",
		Moves: []Move{{
			From:         source,
			To:           occupied,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("ORIGINAL\n"))},
		}},
	}); !errors.Is(err, ErrMoveTargetExists) {
		t.Fatalf("error = %v, want ErrMoveTargetExists", err)
	}

	_, err := manager.Commit(Request{
		Operation: "key.trash",
		Moves: []Move{{
			From:         source,
			To:           filepath.Join(workspace.Root(), "moved"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("SOMETHING ELSE\n"))},
		}},
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want a ConflictError", err)
	}
	if conflict.Current != nil {
		t.Fatalf("a conflict on a move carried file contents, which may be key material")
	}
	if contents, readErr := os.ReadFile(source); readErr != nil || string(contents) != "ORIGINAL\n" {
		t.Fatalf("source changed after a rejected move: %q, %v", contents, readErr)
	}
}

func TestCommitRemovesAFileWithoutWritingABackup(t *testing.T) {
	manager, workspace := newTestManager(t)
	target := writeWorkspaceFile(t, workspace, "sshc/trash/entry-1/id_work", "PRIVATE KEY BYTES\n", 0o600)

	result, err := manager.Commit(Request{
		Operation: "key.purge",
		Removals: []Removal{{
			Path:         target,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY BYTES\n"))},
		}},
	})
	if err != nil {
		t.Fatalf("Commit error = %v", err)
	}
	if _, statErr := os.Lstat(target); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("removed file still exists: %v", statErr)
	}
	if entries, readErr := os.ReadDir(result.BackupDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("a permanent delete wrote a backup: %#v", entries)
	}
}

func TestNoteRecordsAnAuditFactWithoutFileContents(t *testing.T) {
	manager, workspace := newTestManager(t)
	target := writeWorkspaceFile(t, workspace, "id_work", "PRIVATE KEY BYTES\n", 0o600)

	if _, err := manager.Note("key.reveal", []string{target}); err != nil {
		t.Fatalf("Note error = %v", err)
	}

	history, err := manager.History()
	if err != nil {
		t.Fatalf("History error = %v", err)
	}
	if len(history) != 1 || history[0].Operation != "key.reveal" {
		t.Fatalf("history = %#v", history)
	}
	if len(history[0].Paths) != 1 || history[0].Paths[0] != target {
		t.Fatalf("history paths = %#v", history[0].Paths)
	}
	if contents, readErr := os.ReadFile(target); readErr != nil || string(contents) != "PRIVATE KEY BYTES\n" {
		t.Fatalf("Note changed the file it recorded: %q, %v", contents, readErr)
	}
	if journalEntries, readErr := os.ReadDir(filepath.Join(workspace.StateDir(), "journal")); readErr == nil && len(journalEntries) != 0 {
		t.Fatalf("Note left a pending journal: %#v", journalEntries)
	}

	records, err := os.ReadDir(filepath.Join(workspace.StateDir(), "history"))
	if err != nil {
		t.Fatalf("read history directory: %v", err)
	}
	for _, entry := range records {
		document, readErr := os.ReadFile(filepath.Join(workspace.StateDir(), "history", entry.Name()))
		if readErr != nil {
			t.Fatalf("read history record: %v", readErr)
		}
		if strings.Contains(string(document), "PRIVATE KEY BYTES") {
			t.Fatalf("the audit record contains file contents")
		}
	}
}

// 秘密鍵が世代バックアップのディレクトリへ複製されることは決してあってはならない。
// そこで、鍵素材を置き換える呼び出し側はバックアップを取らないことを選び、その
// 変更があとから巻き戻せないことを受け入れる。
func TestCommitSkipsTheGenerationalBackupWhenTheCallerOptsOut(t *testing.T) {
	manager, workspace := newTestManager(t)
	secret := writeWorkspaceFile(t, workspace, "id_work", "PRIVATE KEY BYTES\n", 0o600)

	result, err := manager.Commit(Request{
		Operation: "key.passphrase",
		Changes: []Change{{
			Path:         secret,
			Contents:     []byte("RE-ENCRYPTED KEY BYTES\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE KEY BYTES\n"))},
			SkipBackup:   true,
		}},
	})
	if err != nil {
		t.Fatalf("Commit error = %v", err)
	}

	contents, err := os.ReadFile(secret)
	if err != nil || string(contents) != "RE-ENCRYPTED KEY BYTES\n" {
		t.Fatalf("contents = %q, %v", contents, err)
	}
	if entries, readErr := os.ReadDir(result.BackupDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("a change that opted out of the backup still wrote one: %#v", entries)
	}

	history, err := manager.History()
	if err != nil {
		t.Fatalf("History error = %v", err)
	}
	if len(history) != 1 || history[0].Operation != "key.passphrase" {
		t.Fatalf("history = %#v", history)
	}
}

func TestCommitCreatesADirectoryAndTheFileInsideItInOneTransaction(t *testing.T) {
	// これがなかった頃は、呼び出し側はジャーナルの外で EnsureDirectory を呼び、
	// mkdir とコミットのあいだでクラッシュすれば空のディレクトリが残ることを
	// 受け入れるしかなかった。
	manager, workspace := newTestManager(t)
	nested := filepath.Join(workspace.Root(), "connections", "work", "eu")

	result, err := manager.Commit(Request{
		Operation:   "test.directory",
		Directories: []DirectoryCreate{{Path: nested}},
		Changes: []Change{{
			Path:     filepath.Join(nested, "lon.conf"),
			Contents: []byte("Host lon-1\n"),
		}},
	})
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
	if result.ID == "" {
		t.Error("no transaction id")
	}

	info, err := os.Stat(nested)
	if err != nil || !info.IsDir() {
		t.Fatalf("the directory was not created: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("mode = %v, want 0700", info.Mode().Perm())
	}
	body, err := os.ReadFile(filepath.Join(nested, "lon.conf"))
	if err != nil || string(body) != "Host lon-1\n" {
		t.Errorf("file = %q, %v", body, err)
	}
}

func TestCommitRemovesAnEmptyDirectoryAndRefusesAFullOne(t *testing.T) {
	// 再帰的な削除は、トランザクションが一度も読んでいない内容を復元しない限り
	// 巻き戻せない。だから消えるのは空のディレクトリだけである。
	manager, workspace := newTestManager(t)
	full := filepath.Join(workspace.Root(), "connections", "work")
	if err := os.MkdirAll(full, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "lon.conf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := manager.Commit(Request{
		Operation:         "test.directory",
		RemoveDirectories: []DirectoryRemoval{{Path: full}},
	})
	if !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("removing a full directory = %v, want ErrDirectoryNotEmpty", err)
	}
	if _, err := os.Stat(filepath.Join(full, "lon.conf")); err != nil {
		t.Errorf("the refused removal touched the contents: %v", err)
	}

	if err := os.Remove(filepath.Join(full, "lon.conf")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(Request{
		Operation:         "test.directory",
		RemoveDirectories: []DirectoryRemoval{{Path: full}},
	}); err != nil {
		t.Fatalf("removing an empty directory = %v", err)
	}
	if _, err := os.Stat(full); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory is still there: %v", err)
	}
}

func TestRemovingADirectoryThatIsAlreadyGoneIsNotAnError(t *testing.T) {
	// これは呼び出し側が求めた状態である。すでに残骸が片付けられているグループ名の
	// 変更が、次のトランザクションを失敗させてはならない。
	manager, workspace := newTestManager(t)

	if _, err := manager.Commit(Request{
		Operation: "test.directory",
		Changes: []Change{{
			Path: filepath.Join(workspace.Root(), "marker"), Contents: []byte("x"),
		}},
		RemoveDirectories: []DirectoryRemoval{
			{Path: filepath.Join(workspace.Root(), "never-existed")},
		},
	}); err != nil {
		t.Fatalf("Commit = %v", err)
	}
}

func TestARefusedDirectoryRequestCreatesNothing(t *testing.T) {
	// バリデータは、ディレクトリが計画されたあと、そのどれかが作られる前に走る。
	// したがって拒否されたリクエストは、ディスクに手を触れずに終わらねばならない。
	manager, workspace := newTestManager(t)
	manager.Validate = func(Request) error { return errors.New("refused") }
	nested := filepath.Join(workspace.Root(), "connections", "work")

	if _, err := manager.Commit(Request{
		Operation:   "test.directory",
		Directories: []DirectoryCreate{{Path: nested}},
	}); err == nil {
		t.Fatal("the validator was ignored")
	}
	if _, err := os.Stat(nested); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused request created the directory: %v", err)
	}
}

func TestADirectoryOutsideTheWorkspaceIsRefused(t *testing.T) {
	manager, _ := newTestManager(t)

	for _, path := range []string{"/etc/ssh", "relative/path", "/"} {
		if _, err := manager.Commit(Request{
			Operation:   "test.directory",
			Directories: []DirectoryCreate{{Path: path}},
		}); err == nil {
			t.Errorf("creating %q was allowed", path)
		}
	}
}

func TestARemovalCanKeepABackupSoHistoryCanRestoreIt(t *testing.T) {
	// ユーザーがエクスプローラから削除した設定ファイルは鍵素材ではないし、この
	// アプリケーションが行う他のすべての変更は History から取り消せる。世代コピーは、
	// 削除を同じ土俵に載せるものだ。Restore が読むのは、まさにこのファイルで
	// ある。
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "conf.d/10-home.conf", "Host nas\n\tUser aida\n", 0o600)

	result, err := manager.Commit(Request{
		Operation: "config.file_delete",
		Removals: []Removal{{
			Path:         path,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host nas\n\tUser aida\n"))},
			Backup:       true,
		}},
	})
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the file is still there: %v", err)
	}

	backup := filepath.Join(result.BackupDir, "conf.d", "10-home.conf")
	kept, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("the removal kept no backup: %v", err)
	}
	if string(kept) != "Host nas\n\tUser aida\n" {
		t.Errorf("backup = %q", kept)
	}
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want the mode the file had", info.Mode().Perm())
	}
}

func TestAPurgeStillCopiesNothingIntoTheBackupDirectory(t *testing.T) {
	// 恒久的な鍵の削除こそ、削除が既定では何も残さない理由である。鍵素材を
	// バックアップディレクトリへコピーすれば、ユーザーがそのために与えた二度の
	// 確認を台無しにしてしまう。
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "keys/id_ed25519", "PRIVATE", 0o600)

	result, err := manager.Commit(Request{
		Operation: "key.purge",
		Removals: []Removal{{
			Path:         path,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("PRIVATE"))},
		}},
	})
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "keys", "id_ed25519")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the purge wrote the key into the backup directory: %v", err)
	}
}

// バックアップは暗号文であり、それを知っているのはマネージャだけである。
//
// ほかにバックアップディレクトリを直接読むものはない。巻き戻しも復元もここへ
// 戻ってくるので、それらのバイト列が何であるかを知る場所はひとつだけであり、
// それを忘れうる呼び出し側は存在しない。
func TestBackupsAreSealedAndReadBackThroughTheManager(t *testing.T) {
	manager, workspace := newTestManager(t)
	manager.Seal = sealForTest
	manager.Unseal = unsealForTest

	path := filepath.Join(workspace.Root(), "config")
	if err := os.WriteFile(path, []byte("Host bastion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{{
			Path:         path,
			Contents:     []byte("Host bastion\n\tPort 2222\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host bastion\n"))},
		}},
	})
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}

	backup := filepath.Join(result.BackupDir, "config")
	onDisk, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read the backup: %v", err)
	}
	if bytes.Equal(onDisk, []byte("Host bastion\n")) {
		t.Error("the backup is the previous contents in the clear")
	}
	restored, err := manager.ReadBackup(backup)
	if err != nil {
		t.Fatalf("ReadBackup = %v", err)
	}
	if string(restored) != "Host bastion\n" {
		t.Errorf("ReadBackup = %q, want the previous contents", restored)
	}
}

// sealForTest は vault の鍵の代役である。可逆であり、かつ明らかに恒等写像では
// ない。ディスク上のバイト列は入ってきたバイト列と違わねばならず、さもなければ
// バックアップを鍵素材で grep する検査が平文の上を素通りしてしまう。
func sealForTest(plaintext []byte) ([]byte, error) {
	sealed := make([]byte, 0, len(plaintext)+len(testSealMarker))
	sealed = append(sealed, testSealMarker...)
	for _, b := range plaintext {
		sealed = append(sealed, b^0x5a)
	}
	return sealed, nil
}

func unsealForTest(sealed []byte) ([]byte, error) {
	if !bytes.HasPrefix(sealed, testSealMarker) {
		return nil, errors.New("that backup was not sealed")
	}
	body := sealed[len(testSealMarker):]
	plaintext := make([]byte, 0, len(body))
	for _, b := range body {
		plaintext = append(plaintext, b^0x5a)
	}
	return plaintext, nil
}

var testSealMarker = []byte("sealed:")

// このリクエスト自身が空にするディレクトリは、このリクエストで取り除ける。
//
// 以前は現状のディスクに対して検査していたので、呼び出し側は一方のトランザクション
// でファイルを外へ移し、次のトランザクションでディレクトリを取り除かねばならな
// かった。つまりグループ名の変更が自分で始めたことを終えられず、二つのあいだで
// クラッシュすれば空の抜け殻が残った。いまは、このリクエストが残すことになる
// ディスクの状態に対して検査する。
func TestADirectoryEmptiedByTheSameRequestIsRemoved(t *testing.T) {
	manager, workspace := newTestManager(t)
	from := writeWorkspaceFile(t, workspace, "connections/work/web.conf", "Host web\n", 0o600)
	nested := writeWorkspaceFile(t, workspace, "connections/work/eu/lon.conf", "Host lon\n", 0o600)

	if _, err := manager.Commit(Request{
		Operation: "config.group_rename",
		Directories: []DirectoryCreate{
			{Path: filepath.Join(workspace.Root(), "connections", "client-a", "eu")},
		},
		Moves: []Move{
			{
				From:         from,
				To:           filepath.Join(workspace.Root(), "connections", "client-a", "web.conf"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host web\n"))},
			},
			{
				From:         nested,
				To:           filepath.Join(workspace.Root(), "connections", "client-a", "eu", "lon.conf"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host lon\n"))},
			},
		},
		// 意図して親を先に列挙している。成立する順序は深いものからであり、呼び出し側が
		// それを知っている必要はない。
		RemoveDirectories: []DirectoryRemoval{
			{Path: filepath.Join(workspace.Root(), "connections", "work")},
			{Path: filepath.Join(workspace.Root(), "connections", "work", "eu")},
		},
	}); err != nil {
		t.Fatalf("Commit = %v", err)
	}

	for _, gone := range []string{"connections/work/eu", "connections/work"} {
		if _, err := os.Stat(filepath.Join(workspace.Root(), filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace.Root(), "connections", "client-a", "eu", "lon.conf")); err != nil {
		t.Errorf("the nested file did not arrive: %v", err)
	}
}

// このリクエストが触れない何かを保持しているディレクトリは、やはり拒否される。
// 変わったのは何をもって空とするかであって、空であることが重要かどうかではない。
func TestADirectoryHoldingSomethingElseIsStillRefused(t *testing.T) {
	manager, workspace := newTestManager(t)
	from := writeWorkspaceFile(t, workspace, "connections/work/web.conf", "Host web\n", 0o600)
	writeWorkspaceFile(t, workspace, "connections/work/notes.txt", "not ours\n", 0o600)

	_, err := manager.Commit(Request{
		Operation: "config.group_rename",
		Directories: []DirectoryCreate{
			{Path: filepath.Join(workspace.Root(), "connections", "client-a")},
		},
		Moves: []Move{{
			From:         from,
			To:           filepath.Join(workspace.Root(), "connections", "client-a", "web.conf"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host web\n"))},
		}},
		RemoveDirectories: []DirectoryRemoval{
			{Path: filepath.Join(workspace.Root(), "connections", "work")},
		},
	})
	if !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("Commit = %v, want ErrDirectoryNotEmpty", err)
	}
	// 何かが起きる前に拒否された。
	if _, statErr := os.Stat(from); statErr != nil {
		t.Errorf("the file moved despite the refusal: %v", statErr)
	}
}

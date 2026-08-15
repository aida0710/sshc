package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoryListsNewestFirstWithoutFileContents(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "config", "Host one\n", 0o600)
	for _, step := range []struct{ previous, next string }{
		{"Host one\n", "Host two\n"},
		{"Host two\n", "Host three\n"},
	} {
		if _, err := manager.Commit(Request{
			Operation: "config.save",
			Changes:   []Change{{Path: path, Contents: []byte(step.next), Precondition: Precondition{Exists: true, Digest: Digest([]byte(step.previous))}}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	history, err := manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %#v", history)
	}
	if !history[0].StartedAt.After(history[1].StartedAt) {
		t.Fatalf("history is not newest first: %v then %v", history[0].StartedAt, history[1].StartedAt)
	}
	if history[0].Operation != "config.save" || len(history[0].Paths) != 1 || history[0].Paths[0] != path {
		t.Fatalf("record = %#v", history[0])
	}
	if history[0].FinishedAt.IsZero() || history[0].BackupDir == "" {
		t.Fatalf("record = %#v", history[0])
	}

	backup, err := manager.workspace.FileSystem().ReadFile(history[0].BackupDir + "/config")
	if err != nil || !bytes.Equal(backup, []byte("Host two\n")) {
		t.Fatalf("backup = %q, %v", backup, err)
	}
}

func TestWriterGeneratedCompletedHistoryFormsRoundTrip(t *testing.T) {
	t.Run("all non-atomic actions and backups", func(t *testing.T) {
		manager, workspace := newTestManager(t)
		changed := writeWorkspaceFile(t, workspace, "config", "before\n", 0o600)
		moveSource := writeWorkspaceFile(t, workspace, "move-source", "move\n", 0o600)
		moveTarget := filepath.Join(workspace.Root(), "move-target")
		removed := writeWorkspaceFile(t, workspace, "remove", "remove\n", 0o600)
		createdDirectory := filepath.Join(workspace.Root(), "created")
		removedDirectory := filepath.Join(workspace.Root(), "empty")
		if err := os.Mkdir(removedDirectory, 0o700); err != nil {
			t.Fatal(err)
		}

		result, err := manager.Commit(Request{
			Operation:   "journal.matrix",
			Directories: []DirectoryCreate{{Path: createdDirectory}},
			Changes: []Change{{
				Path:         changed,
				Contents:     []byte("after\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
			}},
			Moves: []Move{{
				From:         moveSource,
				To:           moveTarget,
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("move\n"))},
			}},
			Removals: []Removal{{
				Path:         removed,
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("remove\n"))},
				Backup:       true,
			}},
			RemoveDirectories: []DirectoryRemoval{{Path: removedDirectory}},
		})
		if err != nil {
			t.Fatalf("Commit matrix = %v", err)
		}
		assertGeneratedHistoryActions(t, manager, []string{
			actionMakeDir, actionWrite, actionMove, actionRemove, actionRemoveDir,
		})
		for _, relative := range []string{"config", "remove"} {
			if _, err := os.Stat(filepath.Join(result.BackupDir, relative)); err != nil {
				t.Fatalf("generated backup %q = %v", relative, err)
			}
		}
	})

	t.Run("atomic completed write", func(t *testing.T) {
		manager, workspace := newTestManager(t)
		target := writeWorkspaceFile(t, workspace, "config", "before\n", 0o600)
		if _, err := manager.CommitAtomic(Request{
			Operation: "journal.atomic",
			Changes: []Change{{
				Path:         target,
				Contents:     []byte("after\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
			}},
		}); err != nil {
			t.Fatalf("CommitAtomic = %v", err)
		}
		assertGeneratedHistoryActions(t, manager, []string{actionWrite})
	})

	t.Run("note", func(t *testing.T) {
		manager, workspace := newTestManager(t)
		target := writeWorkspaceFile(t, workspace, "config", "Host test\n", 0o600)
		if _, err := manager.Note("config.inspect", []string{target}); err != nil {
			t.Fatalf("Note = %v", err)
		}
		assertGeneratedHistoryActions(t, manager, []string{actionNote})
	})
}

func assertGeneratedHistoryActions(t *testing.T, manager *Manager, want []string) {
	t.Helper()
	records, err := manager.readRecords(manager.historyDirectory())
	if err != nil || len(records) != 1 {
		t.Fatalf("generated history records = %#v, %v", records, err)
	}
	got := make([]string, 0, len(records[0].Entries))
	for _, entry := range records[0].Entries {
		got = append(got, entry.Action)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generated actions = %#v, want %#v", got, want)
	}
	if records[0].Status != statusCompleted || records[0].FinishedAt == nil || records[0].Committed != len(records[0].Entries) {
		t.Fatalf("generated completed record = %#v", records[0])
	}
	if history, err := manager.History(); err != nil || len(history) != 1 {
		t.Fatalf("History = %#v, %v", history, err)
	}
}

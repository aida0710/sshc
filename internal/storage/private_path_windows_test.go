//go:build windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsPrivateStateContainmentIsCaseInsensitive(t *testing.T) {
	for _, state := range []string{
		filepath.Join(`C:\Users\Aida\.ssh`, "sshc"),
		filepath.Join(`\\Server\Share\Users\Aida\.ssh`, "sshc"),
	} {
		t.Run(state, func(t *testing.T) {
			for name, path := range map[string]string{
				"same directory": strings.ToUpper(state),
				"descendant":     strings.ToUpper(filepath.Join(state, "trash", "entry", "id_work")),
			} {
				t.Run(name, func(t *testing.T) {
					if !privateStateContains(state, path) {
						t.Fatalf("privateStateContains(%q, %q) = false", state, path)
					}
				})
			}
			if privateStateContains(state, strings.ToUpper(state)+"-outside") {
				t.Fatal("case-insensitive prefix without a path boundary was accepted")
			}
		})
	}
}

func TestWindowsWritersRejectCaseAliasDuplicatesBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Manager, *Workspace) error
	}{
		{
			name: "Commit",
			call: func(manager *Manager, workspace *Workspace) error {
				_, err := manager.Commit(Request{
					Operation: "directory.create",
					Directories: []DirectoryCreate{
						{Path: filepath.Join(workspace.Root(), "Keys")},
						{Path: filepath.Join(strings.ToLower(workspace.Root()), "keys")},
					},
				})
				return err
			},
		},
		{
			name: "Note",
			call: func(manager *Manager, workspace *Workspace) error {
				path := filepath.Join(workspace.Root(), "config")
				if err := os.WriteFile(path, []byte("Host test\n"), 0o600); err != nil {
					return err
				}
				_, err := manager.Note("config.inspect", []string{path, strings.ToUpper(path)})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, workspace := newTestManager(t)
			if err := test.call(manager, workspace); !errors.Is(err, ErrDuplicatePath) {
				t.Fatalf("case-alias %s = %v, want ErrDuplicatePath", test.name, err)
			}
			if _, err := os.Stat(workspace.StateDir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("case-alias %s mutated state: %v", test.name, err)
			}
		})
	}
}

func TestWindowsLoadedJournalRejectsCaseAliasPathAndTarget(t *testing.T) {
	root := `C:\Users\Aida\.ssh`
	manager := &Manager{workspace: &Workspace{root: root}}
	digest := Digest([]byte("value"))
	record := journalRecord{
		ID:        validJournalTestID,
		Version:   journalVersion,
		Operation: "key.move",
		Status:    statusStaged,
		StartedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		Entries: []journalEntry{{
			Action:         actionMove,
			Path:           filepath.Join(root, "Keys", "id_work"),
			Target:         filepath.Join(strings.ToLower(root), "keys", "ID_WORK"),
			HadPrevious:    true,
			Mode:           0o600,
			Digest:         digest,
			PreviousDigest: digest,
		}},
	}
	if err := manager.validateLoadedJournalRecord(record, validJournalTestID+".json", manager.journalDirectory()); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("case-alias Path/Target journal = %v, want ErrInvalidJournal", err)
	}
}

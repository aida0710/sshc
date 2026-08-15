//go:build windows

package storage

import (
	"errors"
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

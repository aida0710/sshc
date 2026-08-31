package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validJournalTestID = "20260816T120000.000-0123abcd"

func TestLoadedJournalValidationRejectsMalformedRecords(t *testing.T) {
	manager, workspace := newTestManager(t)
	valid := validPendingJournalRecord(workspace)
	outside := filepath.Join(filepath.Dir(workspace.Root()), "outside-canary")

	tests := []struct {
		name   string
		change func(*journalRecord)
	}{
		{"filename and id mismatch", func(record *journalRecord) { record.ID = "20260816T120000.000-deadbeef" }},
		{"unsupported version", func(record *journalRecord) { record.Version++ }},
		{"empty operation", func(record *journalRecord) { record.Operation = "" }},
		{"unknown status", func(record *journalRecord) { record.Status = "unknown" }},
		{"completed status in current journal", func(record *journalRecord) {
			finished := record.StartedAt.Add(time.Second)
			record.Status = statusCompleted
			record.FinishedAt = &finished
			record.Committed = len(record.Entries)
			record.Entries[0].Temp = ""
		}},
		{"rolled-back status in current journal", func(record *journalRecord) {
			finished := record.StartedAt.Add(time.Second)
			record.Status = statusRolledBack
			record.FinishedAt = &finished
			record.Committed = 0
		}},
		{"pending finished time", func(record *journalRecord) { finished := time.Now(); record.FinishedAt = &finished }},
		{"negative committed", func(record *journalRecord) { record.Committed = -1 }},
		{"committed beyond entries", func(record *journalRecord) { record.Committed = len(record.Entries) + 1 }},
		{"empty action", func(record *journalRecord) { record.Entries[0].Action = "" }},
		{"unknown action", func(record *journalRecord) { record.Entries[0].Action = "execute" }},
		{"outside path", func(record *journalRecord) { record.Entries[0].Path = outside }},
		{"outside target", func(record *journalRecord) { record.Entries[0].Target = outside }},
		{"outside temp", func(record *journalRecord) { record.Entries[0].Temp = outside }},
		{"outside backup", func(record *journalRecord) { record.Entries[0].Backup = outside }},
		{"wrong temp prefix", func(record *journalRecord) {
			record.Entries[0].Temp = filepath.Join(workspace.Root(), ".sshc-another-id")
		}},
		{"bad digest", func(record *journalRecord) { record.Entries[0].Digest = "not-a-digest" }},
		{"broad file mode", func(record *journalRecord) { record.Entries[0].Mode = 0o644 }},
		{"atomic move", func(record *journalRecord) {
			record.Atomic = true
			record.Entries[0] = journalEntry{
				Action:         actionMove,
				Path:           filepath.Join(workspace.Root(), "source"),
				Target:         filepath.Join(workspace.Root(), "target"),
				HadPrevious:    true,
				Mode:           0o600,
				Digest:         Digest([]byte("value")),
				PreviousDigest: Digest([]byte("value")),
			}
		}},
		{"atomic write without backup", func(record *journalRecord) {
			record.Atomic = true
			record.Entries[0].NoBackup = true
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := cloneJournalRecord(valid)
			test.change(&record)
			err := manager.validateLoadedJournalRecord(record, validJournalTestID+".json", manager.journalDirectory())
			if !errors.Is(err, ErrInvalidJournal) {
				t.Fatalf("validateLoadedJournalRecord = %v, want ErrInvalidJournal", err)
			}
		})
	}
}

func TestLoadedJournalMigratesV1WriteModes(t *testing.T) {
	record := journalRecord{
		Version: 1,
		Entries: []journalEntry{{
			Action: actionWrite, HadPrevious: true,
			Mode: 0o600, Digest: Digest([]byte("same")), PreviousDigest: Digest([]byte("same")),
		}},
	}
	if err := migrateLoadedJournalRecord(&record); err != nil {
		t.Fatal(err)
	}
	if record.Version != journalVersion || record.Entries[0].PreviousMode != 0o600 {
		t.Fatalf("migrated record = %#v", record)
	}
}

func TestLoadedJournalRejectsV1RecordWithV2ModeField(t *testing.T) {
	record := journalRecord{
		Version: 1,
		Entries: []journalEntry{{
			Action: actionWrite, HadPrevious: true,
			Mode: 0o700, PreviousMode: 0o600,
		}},
	}
	if err := migrateLoadedJournalRecord(&record); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("migration = %v, want ErrInvalidJournal", err)
	}
}

func TestJournalRecoveryRejectsOutsidePathsBeforeTouchingThem(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*journalRecord, string)
		call   func(*Manager) error
	}{
		{
			name: "Pending before atomic reconcile",
			change: func(record *journalRecord, outside string) {
				record.Atomic = true
				record.Entries[0].Path = outside
			},
			call: func(manager *Manager) error { _, err := manager.Pending(); return err },
		},
		{
			name: "Complete before remove",
			change: func(record *journalRecord, outside string) {
				record.Entries[0] = journalEntry{
					Action:         actionRemove,
					Path:           outside,
					HadPrevious:    true,
					NoBackup:       true,
					Mode:           0o600,
					Digest:         Digest([]byte("outside-canary")),
					PreviousDigest: Digest([]byte("outside-canary")),
				}
			},
			call: func(manager *Manager) error { return manager.Complete(validJournalTestID) },
		},
		{
			name: "Rollback before remove",
			change: func(record *journalRecord, outside string) {
				record.Committed = 1
				record.Entries[0] = journalEntry{
					Action:      actionWrite,
					Path:        outside,
					HadPrevious: false,
					Mode:        0o600,
					Digest:      Digest([]byte("replacement")),
				}
			},
			call: func(manager *Manager) error { return manager.Rollback(validJournalTestID) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, workspace := newTestManager(t)
			outside := filepath.Join(filepath.Dir(workspace.Root()), "outside-canary")
			if err := os.WriteFile(outside, []byte("outside-canary"), 0o600); err != nil {
				t.Fatal(err)
			}
			record := validPendingJournalRecord(workspace)
			test.change(&record, outside)
			if err := workspace.EnsureDirectory(manager.journalDirectory()); err != nil {
				t.Fatal(err)
			}
			journalPath := filepath.Join(manager.journalDirectory(), validJournalTestID+".json")
			if err := manager.writeRecord(journalPath, record); err != nil {
				t.Fatal(err)
			}
			assertLoadedJournalFixturePolicy(t, journalPath)

			if err := test.call(manager); !errors.Is(err, ErrInvalidJournal) {
				t.Fatalf("recovery = %v, want ErrInvalidJournal", err)
			}
			contents, err := os.ReadFile(outside)
			if err != nil || string(contents) != "outside-canary" {
				t.Fatalf("outside canary = %q, %v", contents, err)
			}
		})
	}
}

func TestReadRecordsRejectsTraversalNameBeforeReadFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &maliciousJournalDirectoryFileSystem{FileSystem: OSFileSystem{}}
	workspace, err := NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspace, time.Now, zeroJournalReader{})

	_, err = manager.readRecords(manager.journalDirectory())
	if !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("readRecords = %v, want ErrInvalidJournal", err)
	}
	if fileSystem.readFileCalls != 0 {
		t.Fatalf("malicious name reached ReadFile %d times", fileSystem.readFileCalls)
	}
}

func TestReadBackupRejectsPathsOutsideTheExpectedBackupTree(t *testing.T) {
	manager, workspace := newTestManager(t)
	outside := filepath.Join(workspace.Root(), "config")
	if _, err := manager.ReadBackup(outside); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("ReadBackup outside backup tree = %v, want ErrInvalidJournal", err)
	}
}

func validPendingJournalRecord(workspace *Workspace) journalRecord {
	return journalRecord{
		ID:        validJournalTestID,
		Version:   journalVersion,
		Operation: "config.save",
		Status:    statusStaged,
		StartedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		Entries: []journalEntry{{
			Action: actionWrite,
			Path:   filepath.Join(workspace.Root(), "config"),
			Temp:   filepath.Join(workspace.Root(), temporaryPrefix+validJournalTestID+"-staged"),
			Mode:   0o600,
			Digest: Digest([]byte("replacement")),
		}},
	}
}

func cloneJournalRecord(record journalRecord) journalRecord {
	cloned := record
	cloned.Entries = append([]journalEntry(nil), record.Entries...)
	return cloned
}

type maliciousJournalDirectoryFileSystem struct {
	FileSystem
	readFileCalls int
}

func (fileSystem *maliciousJournalDirectoryFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{maliciousJournalEntry{name: "../outside.json"}}, nil
}

func (fileSystem *maliciousJournalDirectoryFileSystem) ReadFile(string) ([]byte, error) {
	fileSystem.readFileCalls++
	return nil, errors.New("unexpected read")
}

type maliciousJournalEntry struct{ name string }

func (entry maliciousJournalEntry) Name() string         { return entry.name }
func (maliciousJournalEntry) IsDir() bool                { return false }
func (maliciousJournalEntry) Type() fs.FileMode          { return 0 }
func (maliciousJournalEntry) Info() (fs.FileInfo, error) { return nil, errors.New("unused") }

type zeroJournalReader struct{}

func (zeroJournalReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = 0
	}
	return len(destination), nil
}

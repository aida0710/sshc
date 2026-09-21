package sftp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

func TestQueueCanReadEverySnapshotItWrites(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "transfers.json")
	manager := NewTransferManager(nil)
	defer manager.Close()
	if err := manager.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	// Each path is under Linux PATH_MAX and each component under NAME_MAX.
	parent := "/" + strings.Repeat(strings.Repeat("a", 200)+"/", 18)
	for index := 0; index < 150; index++ {
		_, err := manager.CreateJob(CreateTransferJob{
			ID: fmt.Sprintf("transfer_audit_%03d", index), BatchID: "batch_audit_001",
			Alias: "target", SourceAlias: "source", SourcePath: parent + "source.txt", RemotePath: parent + "target.txt",
			Operation: RemoteCopy, Direction: TransferRemote, Kind: TransferFile, Name: "source.txt", TotalBytes: 1,
		})
		if err != nil {
			t.Fatalf("create %d: %v", index, err)
		}
	}
	stored, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewTransferManager(nil)
	defer restarted.Close()
	if err := restarted.EnableQueuePersistence(filename); err != nil {
		t.Fatalf("engine cannot reopen its own %d-byte queue: %v", stored.Size(), err)
	}
}

func TestOversizedQueueIsPreservedWithoutBlockingStartup(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "transfers.json")
	if err := writeQueueAtomically(filename, []byte("legacy queue")); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(filename, maxTransferQueueBytes+1); err != nil {
		t.Fatal(err)
	}
	manager := NewTransferManager(nil)
	defer manager.Close()
	if err := manager.EnableQueuePersistence(filename); err != nil {
		t.Fatalf("restore oversized queue: %v", err)
	}
	preserved, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".sshc-transfers.json.corrupt-*"))
	if err != nil || len(preserved) != 1 {
		t.Fatalf("preserved queue: %v, %v", preserved, err)
	}
	previous, err := os.Stat(preserved[0])
	if err != nil || previous.Size() != maxTransferQueueBytes+1 {
		t.Fatalf("oversized queue was not preserved: %v", err)
	}
	active, err := os.Stat(filename)
	if err != nil || active.Size() >= maxTransferQueueBytes {
		t.Fatalf("new queue is not loadable: %v", err)
	}
}

func TestQueueCapacityFailurePreservesTheSavedSnapshot(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "transfers.json")
	previous := []byte(`{"schemaVersion":1,"jobs":[]}`)
	if err := writeQueueAtomically(filename, previous); err != nil {
		t.Fatal(err)
	}
	if err := writeQueueAtomically(filename, make([]byte, maxTransferQueueBytes+1)); !errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatalf("oversized write: %v", err)
	}
	stored, err := os.ReadFile(filename)
	if err != nil || string(stored) != string(previous) {
		t.Fatalf("failed write changed the snapshot: %v", err)
	}
}

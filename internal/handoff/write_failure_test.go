package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRemovesTheTemporaryFileWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	renameFailure := errors.New("rename failed")
	operations := defaultWriteOperations()
	operations.replace = func(string, string) error { return renameFailure }

	err := write(directory, Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret for the failed replacement",
		Owner:           OwnerEngine,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}, operations)
	if !errors.Is(err, renameFailure) {
		t.Fatalf("Write = %v, want rename failure", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "."+FileName+".tmp-") {
			t.Errorf("temporary file was left behind: %q", entry.Name())
		}
	}
}

func TestWriteDoesNotPublishWhenPrivateTempCreationFails(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("private temp creation failed")
	operations := defaultWriteOperations()
	operations.createTemp = func(string, string) (*os.File, error) { return nil, want }

	err := write(directory, Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret that must not be published",
		Owner:           OwnerEngine,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}, operations)
	if !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == FileName || strings.HasPrefix(entry.Name(), "."+FileName+".tmp-") {
			t.Fatalf("failed private creation published %q", entry.Name())
		}
	}
}

func TestWriteReportsTheDirectoryDurabilityFailureAfterReplacement(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("directory durability failed")
	operations := defaultWriteOperations()
	operations.syncDirectory = func(string) error { return want }

	err := write(directory, Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret for the durability failure",
		Owner:           OwnerEngine,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}, operations)
	if !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	if _, err := os.Stat(filepath.Join(directory, FileName)); err != nil {
		t.Fatalf("replacement did not precede directory durability: %v", err)
	}
}

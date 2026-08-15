package handoff

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWriteRemovesTheTemporaryFileWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	renameFailure := errors.New("rename failed")
	originalRename := renameFile
	renameFile = func(string, string) error { return renameFailure }
	t.Cleanup(func() { renameFile = originalRename })

	err := Write(directory, Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret for the failed replacement",
		Owner:           OwnerHeadless,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	})
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

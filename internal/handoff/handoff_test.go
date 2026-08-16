package handoff_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/handoff"
)

func validDocument() handoff.Handoff {
	return handoff.Handoff{
		SchemaVersion:   handoff.SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "a secret for exactly this running engine",
		Owner:           handoff.OwnerHeadless,
		PID:             4242,
		Version:         "v1.2.3",
		ProtocolVersion: handoff.ProtocolVersion,
	}
}

// 旧形式を受け入れると、所有者と互換性を確かめられないまま別版のエンジンへ接続する。
func TestReadRejectsTheLegacyURLAndSecretDocument(t *testing.T) {
	directory := writeHandoffFixture(t, []byte(`{"url":"http://127.0.0.1:52865","secret":"old"}`))

	_, err := handoff.Read(directory)
	if !errors.Is(err, handoff.ErrSchemaVersion) {
		t.Fatalf("Read legacy document = %v, want schema-version error", err)
	}
}

func TestReadRejectsInvalidHandoffFields(t *testing.T) {
	tests := []struct {
		name   string
		change func(*handoff.Handoff)
		want   error
	}{
		{"unknown owner", func(document *handoff.Handoff) { document.Owner = "other" }, handoff.ErrInvalid},
		{"schema mismatch", func(document *handoff.Handoff) { document.SchemaVersion = 2 }, handoff.ErrSchemaVersion},
		{"protocol mismatch", func(document *handoff.Handoff) { document.ProtocolVersion = 2 }, handoff.ErrProtocolVersion},
		{"non-loopback URL", func(document *handoff.Handoff) { document.URL = "http://example.com:52865" }, handoff.ErrInvalid},
		{"empty secret", func(document *handoff.Handoff) { document.Secret = "" }, handoff.ErrInvalid},
		{"zero PID", func(document *handoff.Handoff) { document.PID = 0 }, handoff.ErrInvalid},
		{"empty version", func(document *handoff.Handoff) { document.Version = "" }, handoff.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := validDocument()
			test.change(&document)
			body, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			directory := writeHandoffFixture(t, body)

			_, err = handoff.Read(directory)
			if !errors.Is(err, test.want) {
				t.Fatalf("Read = %v, want %v", err, test.want)
			}
		})
	}
}

func TestWriteAtomicallyPublishesOnePrivateValidatedDocument(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state", "sshc")
	document := validDocument()
	if err := handoff.Write(directory, document); err != nil {
		t.Fatalf("Write = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(directory, handoff.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded handoff.Handoff
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("published JSON does not decode: %v", err)
	}
	if decoded != document {
		t.Errorf("decoded = %#v, want %#v", decoded, document)
	}
	info, err := os.Stat(filepath.Join(directory, handoff.FileName))
	if err != nil {
		t.Fatal(err)
	}
	// Windows の Chmod は所有者の書き込みビットしか写さないので、この二つの mode
	// は向こうでは何の約束も運ばない。同じ「本人以外は触れない」を Windows で
	// 確かめているのは permissions_windows_test.go の
	// TestWriteRestrictsWindowsHandoffState であり、そちらは DACL を見ている。
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 0600", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && directoryInfo.Mode().Perm() != 0o700 {
		t.Errorf("directory mode = %o, want 0700", directoryInfo.Mode().Perm())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	foundDocument := false
	for _, entry := range entries {
		if entry.Name() == handoff.FileName {
			foundDocument = true
		}
		if strings.HasPrefix(entry.Name(), "."+handoff.FileName+".tmp-") {
			t.Errorf("temporary file was left behind: %q", entry.Name())
		}
	}
	if !foundDocument {
		t.Errorf("entries = %#v, want %q", entries, handoff.FileName)
	}
}

func TestRemoveOnlyDeletesTheMatchingSecret(t *testing.T) {
	directory := t.TempDir()
	document := validDocument()
	if err := handoff.Write(directory, document); err != nil {
		t.Fatal(err)
	}

	if err := handoff.Remove(directory, "another run's secret"); err != nil {
		t.Fatalf("Remove another secret = %v", err)
	}
	if got, err := handoff.Read(directory); err != nil || got != document {
		t.Fatalf("Read after mismatched Remove = %#v, %v", got, err)
	}
	if err := handoff.Remove(directory, document.Secret); err != nil {
		t.Fatalf("Remove matching secret = %v", err)
	}
	if _, err := handoff.Read(directory); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read after Remove = %v, want missing file", err)
	}
}

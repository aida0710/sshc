package application

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/storage"
	"sshc/internal/textencoding"
)

func TestConnectionEncodingFollowsTheConcreteHostMetadata(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Host legacy\n  HostName example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager)
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "legacy"}, Encoding: string(textencoding.ShiftJIS),
	}}
	change, err := service.metadata.Change(metadata, storage.Precondition{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(storage.Request{Operation: "test", Changes: []storage.Change{change}}); err != nil {
		t.Fatal(err)
	}

	got, err := service.ConnectionEncoding("LEGACY")
	if err != nil {
		t.Fatal(err)
	}
	if got != textencoding.ShiftJIS {
		t.Fatalf("encoding = %q", got)
	}
	defaulted, err := service.ConnectionEncoding("other")
	if err != nil {
		t.Fatal(err)
	}
	if defaulted != textencoding.UTF8 {
		t.Fatalf("default encoding = %q", defaulted)
	}
}

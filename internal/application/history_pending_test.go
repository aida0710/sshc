package application

import (
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// renameRefusingFileSystem は、refused への置き換えだけを断る。中断した変更を残すために使う。
type renameRefusingFileSystem struct {
	storage.FileSystem
	refused string
}

var errInjectedRename = errors.New("injected rename failure")

func (f renameRefusingFileSystem) Rename(oldPath, newPath string) error {
	if newPath == f.refused {
		return errInjectedRename
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f renameRefusingFileSystem) MovePrivate(oldPath, newPath string) error {
	if newPath == f.refused {
		return errInjectedRename
	}
	return f.FileSystem.MovePrivate(oldPath, newPath)
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"

// interruptedChangeOfEveryKind は、ssh の設定、秘密鍵、ローカルの Vault の鍵、metadata.json、
// 文字でないファイルを1つの変更で書き、metadata.json の置き換えで止めた Service を返す。
// 先頭の3つは書き終え、残りの2つはまだ書いていない。
func interruptedChangeOfEveryKind(t *testing.T) (*Service, *storage.Manager, string) {
	t.Helper()
	home := t.TempDir()
	setup, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := setup.EnsureDirectory(filepath.Join(setup.StateDir(), "backgrounds")); err != nil {
		t.Fatal(err)
	}
	path := func(relative string) string { return filepath.Join(setup.Root(), filepath.FromSlash(relative)) }
	before := map[string]string{
		"config":                 "Host bastion\n\tPort 22\n",
		"id_test":                testPrivateKey,
		"sshc/local-vault-key":   "old key",
		"sshc/metadata.json":     "{\"tags\": [\"prod\"]}\n",
		"sshc/backgrounds/a.bin": "image\x00one",
	}
	after := map[string]string{
		"config":                 "Host bastion\n\tPort 2233\n",
		"id_test":                testPrivateKey + "\n",
		"sshc/local-vault-key":   "new key",
		"sshc/metadata.json":     "{\"tags\": [\"prod\", \"jump\"]}\n",
		"sshc/backgrounds/a.bin": "image\x00two",
	}
	order := []string{"config", "id_test", "sshc/local-vault-key", "sshc/metadata.json", "sshc/backgrounds/a.bin"}
	setupManager := storage.NewManager(setup, time.Now, rand.Reader)
	var creates []storage.Change
	for _, relative := range order {
		creates = append(creates, storage.Change{Path: path(relative), Contents: []byte(before[relative])})
	}
	if _, err := setupManager.Commit(storage.Request{Operation: "test.setup", Changes: creates}); err != nil {
		t.Fatal(err)
	}

	workspace, err := storage.NewWorkspace(renameRefusingFileSystem{FileSystem: storage.OSFileSystem{}, refused: path("sshc/metadata.json")}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	var changes []storage.Change
	for _, relative := range order {
		changes = append(changes, storage.Change{
			Path:         path(relative),
			Contents:     []byte(after[relative]),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte(before[relative]))},
		})
	}
	if _, err := manager.Commit(storage.Request{Operation: "config.save", Changes: changes}); !errors.Is(err, errInjectedRename) {
		t.Fatalf("commit = %v, want the injected rename failure", err)
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	return NewService(workspace, manager), manager, pending[0].ID
}

func TestPendingChangesShowTextAndKeepSecretFilesHidden(t *testing.T) {
	service, _, identifier := interruptedChangeOfEveryKind(t)

	view, err := service.PendingChanges(identifier)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		path      string
		committed bool
		contents  string
	}{
		{"config", true, pendingContentsText},
		{"id_test", true, pendingContentsSecret},
		{"sshc/local-vault-key", true, pendingContentsSecret},
		{"sshc/metadata.json", false, pendingContentsText},
		{"sshc/backgrounds/a.bin", false, pendingContentsSecret},
	}
	if len(view.Files) != len(want) {
		t.Fatalf("files = %#v, want %d", view.Files, len(want))
	}
	for index, expected := range want {
		file := view.Files[index]
		if file.Path != expected.path || file.Committed != expected.committed || file.Contents != expected.contents {
			t.Errorf("file %d = %s committed=%v contents=%s, want %s committed=%v contents=%s",
				index, file.Path, file.Committed, file.Contents, expected.path, expected.committed, expected.contents)
		}
		if file.Contents != pendingContentsText && (file.Before != "" || file.After != "") {
			t.Errorf("%s carries contents although they are hidden as %s", file.Path, file.Contents)
		}
	}
	if config := view.Files[0]; config.Before != "Host bastion\n\tPort 22\n" || config.After != "Host bastion\n\tPort 2233\n" {
		t.Errorf("config before/after = %q / %q", config.Before, config.After)
	}
	if metadata := view.Files[3]; !strings.Contains(metadata.Before, "\"prod\"]") || !strings.Contains(metadata.After, "\"jump\"") {
		t.Errorf("metadata before/after = %q / %q", metadata.Before, metadata.After)
	}
}

func TestPendingChangesSayTheVaultIsLockedWhenTheBackupCannotBeOpened(t *testing.T) {
	service, manager, identifier := interruptedChangeOfEveryKind(t)
	manager.Unseal = func([]byte) ([]byte, error) { return nil, secret.ErrLocked }

	view, err := service.PendingChanges(identifier)
	if err != nil {
		t.Fatal(err)
	}
	if config := view.Files[0]; config.Contents != pendingContentsVaultLocked || config.Before != "" || config.After != "" {
		t.Fatalf("config = %+v, want vault_locked without contents", config)
	}
	// まだ書いていないファイルは控えを使わないので、ロック中でも比べられる。
	if metadata := view.Files[3]; metadata.Contents != pendingContentsText {
		t.Fatalf("metadata = %+v, want text", metadata)
	}
}

func TestPendingChangesCallATextFileWithANulByteBinary(t *testing.T) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader))
	item := storage.PendingContents{
		Path:   filepath.Join(workspace.Root(), "notes.conf"),
		Action: "write",
		Before: storage.PendingSide{Exists: true, Contents: []byte("plain\n")},
		After:  storage.PendingSide{Exists: true, Contents: []byte("plain\x00\n")},
	}
	if kind := service.pendingContentsKind(item); kind != pendingContentsBinary {
		t.Fatalf("kind = %s, want binary", kind)
	}
	item.After.Contents = []byte(strings.Repeat("a", maxPendingDiffBytes+1))
	if kind := service.pendingContentsKind(item); kind != pendingContentsTooLarge {
		t.Fatalf("kind = %s, want too_large", kind)
	}
}

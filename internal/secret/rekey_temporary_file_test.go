package secret_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// マスターパスワードの変更がバックアップを封じ直している途中で落ちると、
// バックアップの隣に一時ファイルが残る。これを封じ直そうとすると、以後の変更が
// 毎回失敗する。一時ファイルはバックアップではないので飛ばす。
func TestChangeMasterPasswordSkipsATemporaryFileLeftAmongTheBackups(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "deploy", "old secret"); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	leftoverDirectory := filepath.Join(workspace.StateDir(), storage.BackupDirectoryName, "20260101T000000.000-deadbeef")
	if err := os.MkdirAll(leftoverDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(leftoverDirectory, ".sshc-20260101T000000.000-deadbeef-123")
	if err := os.WriteFile(leftover, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := service.ChangeMasterPassword(context.Background(), passphrase, "a new master password after a crash"); err != nil {
		t.Fatalf("ChangeMasterPassword with a leftover temporary file = %v", err)
	}
}

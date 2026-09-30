package vpnprofile_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// failingVaultRename は、Vault のファイルへの置き換えを1回だけ失敗させる。metadata を
// 置き換えたあとの、書き込みの途中の失敗を作る。
type failingVaultRename struct {
	storage.OSFileSystem
	vaultPath string
	failure   error
}

func (fileSystem *failingVaultRename) Rename(from, to string) error {
	if fileSystem.failure != nil && filepath.Clean(to) == filepath.Clean(fileSystem.vaultPath) {
		failure := fileSystem.failure
		fileSystem.failure = nil
		return failure
	}
	return fileSystem.OSFileSystem.Rename(from, to)
}

// 改名の書き込みが Vault を置き換える途中で失敗しても、metadata・Vault・保留の記録は
// どれも元のままになる。次の保存も止まらない。
func TestARenameThatFailsHalfwayLeavesEverythingAsItWas(t *testing.T) {
	fileSystem := &failingVaultRename{}
	f := newFixtureOn(t, fileSystem)
	f.create(t, labProfile(), labSecrets())
	if _, err := f.config.SetConnectionVPN("lab", "lab"); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("the vault could not be replaced")
	fileSystem.vaultPath = filepath.Join(f.workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	fileSystem.failure = injected

	if err := f.profiles.Rename(context.Background(), "lab", "tains"); !errors.Is(err, injected) {
		t.Fatalf("Rename = %v", err)
	}

	if names := f.profileNames(t); len(names) != 1 || names[0] != "lab" {
		t.Fatalf("profiles = %v", names)
	}
	if bound, err := f.config.ConnectionVPN("lab"); err != nil || bound != "lab" {
		t.Fatalf("ConnectionVPN = %q, %v", bound, err)
	}
	if got := f.storedSecrets(t, "lab"); got.WireGuardConfig != labSecrets().WireGuardConfig {
		t.Fatalf("stored = %+v", got)
	}
	if pending, err := f.transactions.Pending(); err != nil || len(pending) != 0 {
		t.Fatalf("Pending = %+v, %v", pending, err)
	}
	if err := f.profiles.Rename(context.Background(), "lab", "tains"); err != nil {
		t.Fatalf("次の改名 = %v", err)
	}
	if got := f.storedSecrets(t, "tains"); got.WireGuardConfig != labSecrets().WireGuardConfig {
		t.Fatalf("改名したあとの stored = %+v", got)
	}
}

//go:build windows

package secret_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

func TestWindowsPrivateVaultJournalBackupAndSyncStateUsesProtectedDACL(t *testing.T) {
	service, home := newService(t)
	sshDirectory := filepath.Join(home, ".ssh")
	original := snapshotSecretParentDACL(t, sshDirectory)
	t.Cleanup(func() { restoreSecretParentDACL(t, sshDirectory, original) })
	installPermissiveSecretParentDACL(t, sshDirectory)

	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.Set("server", "password"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "bucket", AccessKeyID: "id", SecretAccessKey: "key"}); err != nil {
		t.Fatal(err)
	}

	stateDirectory := filepath.Join(sshDirectory, "sshc")
	backupRoot := filepath.Join(stateDirectory, storage.BackupDirectoryName)
	backupIDs, err := os.ReadDir(backupRoot)
	if err != nil || len(backupIDs) == 0 {
		t.Fatalf("backup entries = %#v, %v", backupIDs, err)
	}
	backupVaultPath := ""
	for _, entry := range backupIDs {
		candidate := filepath.Join(backupRoot, entry.Name(), "secrets")
		if _, err := os.Stat(candidate); err == nil {
			backupVaultPath = candidate
			break
		}
	}
	if backupVaultPath == "" {
		t.Fatal("no vault backup was written")
	}
	historyEntries, err := os.ReadDir(filepath.Join(stateDirectory, "history"))
	if err != nil || len(historyEntries) == 0 {
		t.Fatalf("history entries = %#v, %v", historyEntries, err)
	}

	for _, path := range []string{
		stateDirectory,
		vaultPath(home),
		filepath.Join(sshDirectory, filepath.FromSlash(secret.SettingsPath)),
		filepath.Join(stateDirectory, "journal"),
		filepath.Join(stateDirectory, "history"),
		filepath.Join(stateDirectory, "history", historyEntries[0].Name()),
		backupRoot,
		filepath.Dir(backupVaultPath),
		backupVaultPath,
	} {
		assertRestrictedSecretPath(t, path)
	}
}

func assertRestrictedSecretPath(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("%q retained a permissive or inherited DACL", path)
	}
}

func installPermissiveSecretParentDACL(t *testing.T, path string) {
	t.Helper()
	userSID := secretCurrentUserSID(t)
	descriptor, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;" + userSID.String() + ")(A;OICI;GRGX;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("permissive DACL = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(descriptor)
}

type secretParentDACL struct {
	descriptor *windows.SECURITY_DESCRIPTOR
	dacl       *windows.ACL
	protected  bool
}

func snapshotSecretParentDACL(t *testing.T, path string) secretParentDACL {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("GetNamedSecurityInfo(%q) = %v", path, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL(%q) = %v", path, err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("Control(%q) = %v", path, err)
	}
	return secretParentDACL{descriptor: descriptor, dacl: dacl, protected: control&windows.SE_DACL_PROTECTED != 0}
}

func restoreSecretParentDACL(t *testing.T, path string, snapshot secretParentDACL) {
	t.Helper()
	information := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	if snapshot.protected {
		information |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		information |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, information, nil, nil, snapshot.dacl, nil); err != nil {
		t.Errorf("restore DACL for %q: %v", path, err)
	}
	runtime.KeepAlive(snapshot.descriptor)
}

func secretCurrentUserSID(t *testing.T) *windows.SID {
	t.Helper()
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

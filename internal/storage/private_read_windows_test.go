//go:build windows

package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

func TestWindowsWorkspacePrivateReadRejectsParentJunction(t *testing.T) {
	workspace := newTestWorkspace(t)
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	targetDirectory := filepath.Join(t.TempDir(), "target")
	if err := windowsacl.EnsureDirectory(targetDirectory); err != nil {
		t.Fatal(err)
	}
	temporary, err := windowsacl.CreateTemp(targetDirectory, ".document-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temporary.Write([]byte("redirected")); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDirectory, "document")
	if err := os.Rename(temporary.Name(), target); err != nil {
		t.Fatal(err)
	}

	junction := filepath.Join(workspace.StateDir(), "redirect")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, targetDirectory).CombinedOutput(); err != nil {
		t.Fatalf("create privilege-free junction fixture: %v: %s", err, output)
	}
	if _, err := workspace.FileSystem().ReadFile(filepath.Join(junction, "document")); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("private read through parent junction = %v, want ErrSymlinkPath", err)
	}
}

func TestWindowsWorkspaceAuthenticatesPrivateStateButNotUserManagedSSHReads(t *testing.T) {
	workspace := newTestWorkspace(t)
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	privatePath := writePrivateStateFixture(t, workspace, "vault", []byte("sealed"))
	if contents, err := workspace.FileSystem().ReadFile(privatePath); err != nil || string(contents) != "sealed" {
		t.Fatalf("authenticated private read = %q, %v", contents, err)
	}

	installPermissiveInheritedDACL(t, privatePath)
	if _, err := workspace.FileSystem().ReadFile(privatePath); !errors.Is(err, windowsacl.ErrInvalidACL) {
		t.Fatalf("permissive private read = %v, want ErrInvalidACL", err)
	}

	managedPath := filepath.Join(workspace.Root(), "config")
	if err := os.WriteFile(managedPath, []byte("Host managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	installPermissiveInheritedDACL(t, managedPath)
	if contents, err := workspace.FileSystem().ReadFile(managedPath); err != nil || string(contents) != "Host managed\n" {
		t.Fatalf("user-managed SSH read = %q, %v", contents, err)
	}
}

func TestWindowsForeignOwnerJournalIsRejectedBeforeEveryRecoveryOperation(t *testing.T) {
	manager, workspace := newTestManager(t)
	outside := filepath.Join(filepath.Dir(workspace.Root()), "outside-canary")
	if err := os.WriteFile(outside, []byte("outside-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(manager.journalDirectory()); err != nil {
		t.Fatal(err)
	}
	record := validPendingJournalRecord(workspace)
	record.Entries[0] = journalEntry{
		Action:         actionRemove,
		Path:           outside,
		HadPrevious:    true,
		NoBackup:       true,
		Mode:           0o600,
		Digest:         Digest([]byte("outside-canary")),
		PreviousDigest: Digest([]byte("outside-canary")),
	}
	journalPath := filepath.Join(manager.journalDirectory(), validJournalTestID+".json")
	if err := manager.writeRecord(journalPath, record); err != nil {
		t.Fatal(err)
	}
	installForeignOwnerExactDACL(t, journalPath)

	for _, operation := range []struct {
		name string
		call func() error
	}{
		{"Pending", func() error { _, err := manager.Pending(); return err }},
		{"Complete", func() error { return manager.Complete(validJournalTestID) }},
		{"Rollback", func() error { return manager.Rollback(validJournalTestID) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, windowsacl.ErrUnexpectedOwner) {
				t.Fatalf("%s = %v, want ErrUnexpectedOwner", operation.name, err)
			}
			contents, err := os.ReadFile(outside)
			if err != nil || string(contents) != "outside-canary" {
				t.Fatalf("outside canary = %q, %v", contents, err)
			}
		})
	}
}

func writePrivateStateFixture(t *testing.T, workspace *Workspace, name string, contents []byte) string {
	t.Helper()
	temporary, err := workspace.FileSystem().WriteTemp(workspace.StateDir(), temporaryPrefix, FilePermission, contents)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.StateDir(), name)
	if err := workspace.FileSystem().Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func installForeignOwnerExactDACL(t *testing.T, path string) {
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
	groups, err := token.GetTokenGroups()
	if err != nil {
		t.Fatal(err)
	}
	var foreignOwner *windows.SID
	for _, group := range groups.AllGroups() {
		if group.Attributes&windows.SE_GROUP_OWNER == 0 || group.Sid == nil || group.Sid.Equals(user.User.Sid) {
			continue
		}
		foreignOwner, err = group.Sid.Copy()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if foreignOwner == nil {
		t.Fatal("Windows token has no distinct SE_GROUP_OWNER SID for the foreign-owner fixture")
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + foreignOwner.String() + "D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)",
	)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		t.Fatalf("foreign owner descriptor = %v", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("foreign owner DACL = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatalf("install foreign-owner fixture: %v", err)
	}
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(groups)
	runtime.KeepAlive(user)
}

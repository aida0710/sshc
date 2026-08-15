//go:build windows

package handoff

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

func TestReadRejectsAWindowsHandoffWithAnExtraAllowACE(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	document := testWindowsHandoff("http://127.0.0.1:52865", "trusted-secret")
	if err := Write(directory, document); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, FileName)
	installExtraAllowACE(t, path)

	if _, err := Read(directory); !errors.Is(err, windowsacl.ErrInvalidACL) {
		t.Fatalf("Read = %v, want windowsacl.ErrInvalidACL", err)
	}
}

func TestReadRejectsAWindowsHandoffOwnedByAnotherTokenSID(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	document := testWindowsHandoff("http://127.0.0.1:52865", "trusted-secret")
	if err := Write(directory, document); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, FileName)
	installForeignOwnerExactDACL(t, path)

	if _, err := Read(directory); !errors.Is(err, windowsacl.ErrUnexpectedOwner) {
		t.Fatalf("Read = %v, want windowsacl.ErrUnexpectedOwner", err)
	}
}

func TestReadUsesTheAuthenticatedWindowsHandleAfterAPathReplacement(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	original := testWindowsHandoff("http://127.0.0.1:52865", "trusted-secret")
	decoy := testWindowsHandoff("http://127.0.0.1:52866", "attacker-secret")
	if err := Write(directory, original); err != nil {
		t.Fatal(err)
	}
	movedPath := filepath.Join(directory, "checked-original")
	operations := defaultHandoffFileOperations()
	originalOpen := operations.open
	operations.open = func(path string) (*os.File, error) {
		file, err := originalOpen(path)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(path, movedPath); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := writeHandoffFixture(path, decoy); err != nil {
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}

	got, err := readValidatedWith(directory, operations.open)
	if err != nil {
		t.Fatalf("readValidatedWith = %v", err)
	}
	if got != original {
		t.Fatalf("readValidatedWith = %#v, want checked original %#v", got, original)
	}
}

func TestRemoveDeletesTheAuthenticatedWindowsHandleAndLeavesAReplacement(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	original := testWindowsHandoff("http://127.0.0.1:52865", "trusted-secret")
	replacement := testWindowsHandoff("http://127.0.0.1:52866", "replacement-secret")
	if err := Write(directory, original); err != nil {
		t.Fatal(err)
	}
	movedPath := filepath.Join(directory, "checked-original")
	operations := defaultHandoffFileOperations()
	originalOpen := operations.open
	operations.open = func(path string) (*os.File, error) {
		file, err := originalOpen(path)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(path, movedPath); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := writeHandoffFixture(path, replacement); err != nil {
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}

	if err := removeWith(directory, original.Secret, operations); err != nil {
		t.Fatalf("removeWith = %v", err)
	}
	if _, err := os.Stat(movedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authenticated original survived removal: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(directory, FileName))
	if err != nil {
		t.Fatalf("replacement path was removed: %v", err)
	}
	var got Handoff
	if err := json.Unmarshal(body, &got); err != nil || got != replacement {
		t.Fatalf("replacement = %#v, %v; want %#v", got, err, replacement)
	}
}

func testWindowsHandoff(url, secret string) Handoff {
	return Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             url,
		Secret:          secret,
		Owner:           OwnerHeadless,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}
}

func writeHandoffFixture(path string, document Handoff) error {
	body, err := json.Marshal(document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func installExtraAllowACE(t *testing.T, path string) {
	t.Helper()
	userSID := handoffCurrentUserSID(t)
	descriptor, err := windows.SecurityDescriptorFromString("O:" + userSID.String() + "D:P(A;;FA;;;" + userSID.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("extra allow DACL = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(descriptor)
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
	userSID := user.User.Sid.String()
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + foreignOwner.String() + "D:P(A;;FA;;;" + userSID + ")(A;;FA;;;SY)(A;;FA;;;BA)",
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

func TestWindowsHandoffReplacementAndDirectoryDurabilityAdapter(t *testing.T) {
	directory := t.TempDir()
	oldPath := filepath.Join(directory, "old")
	newPath := filepath.Join(directory, "new")
	if err := os.WriteFile(oldPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceHandoffFile(oldPath, newPath); err != nil {
		t.Fatalf("replaceHandoffFile = %v", err)
	}
	if err := syncHandoffDirectory(directory); err != nil {
		t.Fatalf("syncHandoffDirectory = %v", err)
	}
	contents, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "replacement" {
		t.Fatalf("replacement contents = %q", contents)
	}
}

func TestWriteRestrictsWindowsHandoffState(t *testing.T) {
	parent := t.TempDir()
	originalParent := snapshotParentDACL(t, parent)
	t.Cleanup(func() { restoreParentDACL(t, parent, originalParent) })
	installPermissiveInheritedParentDACL(t, parent)

	directory := filepath.Join(parent, "state")
	document := Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "handoff-secret",
		Owner:           OwnerHeadless,
		PID:             4242,
		Version:         "v1.2.3",
		ProtocolVersion: ProtocolVersion,
	}
	if err := Write(directory, document); err != nil {
		t.Fatalf("Write = %v", err)
	}

	for _, path := range []string{
		directory,
		filepath.Join(directory, FileName),
		filepath.Join(directory, mutationLockName),
	} {
		assertRestrictedWindowsPath(t, path)
	}
}

func assertRestrictedWindowsPath(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("%q retained a permissive or inherited DACL", path)
	}
}

func installPermissiveInheritedParentDACL(t *testing.T, path string) {
	t.Helper()
	userSID := handoffCurrentUserSID(t)
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

type parentDACL struct {
	descriptor *windows.SECURITY_DESCRIPTOR
	dacl       *windows.ACL
	protected  bool
}

func snapshotParentDACL(t *testing.T, path string) parentDACL {
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
	return parentDACL{descriptor: descriptor, dacl: dacl, protected: control&windows.SE_DACL_PROTECTED != 0}
}

func restoreParentDACL(t *testing.T, path string, snapshot parentDACL) {
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

func handoffCurrentUserSID(t *testing.T) *windows.SID {
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

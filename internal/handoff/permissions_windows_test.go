//go:build windows

package handoff

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

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

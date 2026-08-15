//go:build windows

package windowsacl

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestRestrictReplacesInheritedPermissiveDACL(t *testing.T) {
	parent := t.TempDir()
	originalParent := snapshotDACL(t, parent)
	t.Cleanup(func() { restoreDACL(t, parent, originalParent) })
	installPermissiveInheritedDACL(t, parent)

	directory := filepath.Join(parent, "state")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "vault")
	if err := os.WriteFile(file, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}

	if restricted, err := IsRestrictedToCurrentUser(directory); err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) before restriction = %v", directory, err)
	} else if restricted {
		t.Fatal("inherited permissive directory was reported restricted")
	}

	if err := RestrictDirectory(directory); err != nil {
		t.Fatalf("RestrictDirectory(%q) = %v", directory, err)
	}
	assertExactlyRestricted(t, directory)

	if err := RestrictFile(file); err != nil {
		t.Fatalf("RestrictFile(%q) = %v", file, err)
	}
	assertExactlyRestricted(t, file)
}

func TestIsRestrictedRejectsAnExtraAllowACE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}
	userSID := testCurrentUserSID(t)
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + userSID.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;BU)")
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

	restricted, err := IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatal(err)
	}
	if restricted {
		t.Fatal("DACL with an extra allow ACE was reported restricted")
	}
}

func assertExactlyRestricted(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("GetNamedSecurityInfo(%q) = %v", path, err)
	}
	defer runtime.KeepAlive(descriptor)

	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("%q DACL is not protected", path)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("DACL(%q) = %v, want concrete DACL", path, err)
	}
	if dacl.AceCount != 3 {
		t.Fatalf("DACL(%q) ACE count = %d, want 3", path, dacl.AceCount)
	}

	expected := []*windows.SID{
		testCurrentUserSID(t),
		wellKnownSID(t, windows.WinLocalSystemSid),
		wellKnownSID(t, windows.WinBuiltinAdministratorsSid),
	}
	matched := make([]bool, len(expected))
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != windows.ACCESS_MASK(0x001f01ff) {
			t.Fatalf("DACL(%q) ACE %d = type %d flags %#x mask %#x, want direct full-access allow", path, index, ace.Header.AceType, ace.Header.AceFlags, ace.Mask)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		found := -1
		for expectedIndex, expectedSID := range expected {
			if !matched[expectedIndex] && sid.Equals(expectedSID) {
				found = expectedIndex
				break
			}
		}
		if found == -1 {
			t.Fatalf("DACL(%q) ACE %d grants an unexpected SID", path, index)
		}
		matched[found] = true
	}
	for index, wasMatched := range matched {
		if !wasMatched {
			t.Fatalf("DACL(%q) is missing required SID entry %d", path, index)
		}
	}

	restricted, err := IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = false, want true", path)
	}
}

func installPermissiveInheritedDACL(t *testing.T, path string) {
	t.Helper()
	userSID := testCurrentUserSID(t)
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

type daclSnapshot struct {
	descriptor *windows.SECURITY_DESCRIPTOR
	dacl       *windows.ACL
	protected  bool
}

func snapshotDACL(t *testing.T, path string) daclSnapshot {
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
	return daclSnapshot{descriptor: descriptor, dacl: dacl, protected: control&windows.SE_DACL_PROTECTED != 0}
}

func restoreDACL(t *testing.T, path string, snapshot daclSnapshot) {
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

func testCurrentUserSID(t *testing.T) *windows.SID {
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

func wellKnownSID(t *testing.T, kind windows.WELL_KNOWN_SID_TYPE) *windows.SID {
	t.Helper()
	sid, err := windows.CreateWellKnownSid(kind)
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

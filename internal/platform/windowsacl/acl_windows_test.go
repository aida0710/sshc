//go:build windows

package windowsacl

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDescriptorRequiresCurrentUserAsOwner(t *testing.T) {
	userSID := testCurrentUserSID(t)
	descriptor, err := windows.SecurityDescriptorFromString("O:BUD:P(A;;FA;;;" + userSID.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		t.Fatal(err)
	}

	restricted, err := isDescriptorRestricted(descriptor, userSID)
	if err != nil {
		t.Fatal(err)
	}
	if restricted {
		t.Fatal("an exact DACL with a non-current owner was reported restricted")
	}
}

func TestRestrictFileHandleCannotBeRedirectedToAPathDecoy(t *testing.T) {
	directory := t.TempDir()
	originalPath := filepath.Join(directory, "original")
	if err := os.WriteFile(originalPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pathUTF16, err := windows.UTF16PtrFromString(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(handle), originalPath)
	if file == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("os.NewFile returned nil")
	}
	defer file.Close()

	movedPath := filepath.Join(directory, "moved-original")
	if err := os.Rename(originalPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	installPermissiveInheritedDACL(t, originalPath)

	if err := restrictFileHandle(file); err != nil {
		t.Fatalf("restrictFileHandle = %v", err)
	}
	assertExactlyRestricted(t, movedPath)
	if restricted, err := IsRestrictedToCurrentUser(originalPath); err != nil {
		t.Fatal(err)
	} else if restricted {
		t.Fatal("the decoy at the old path was restricted instead of the open object")
	}
}

func TestPrivateObjectsAreRestrictedWhenCreationReturns(t *testing.T) {
	parent := t.TempDir()
	originalParent := snapshotDACL(t, parent)
	t.Cleanup(func() { restoreDACL(t, parent, originalParent) })
	installPermissiveInheritedDACL(t, parent)

	privateParent := filepath.Join(parent, "private-parent")
	directory := filepath.Join(privateParent, "state")
	if err := EnsureDirectory(directory); err != nil {
		t.Fatalf("EnsureDirectory = %v", err)
	}
	assertExactlyRestricted(t, privateParent)
	assertExactlyRestricted(t, directory)

	temporary, err := CreateTemp(directory, ".secret-")
	if err != nil {
		t.Fatalf("CreateTemp = %v", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	assertExactlyRestricted(t, temporaryPath)
	if info, err := temporary.Stat(); err != nil {
		t.Fatal(err)
	} else if info.Size() != 0 {
		t.Fatalf("new private temp size = %d, want 0", info.Size())
	}

	lockPath := filepath.Join(directory, ".lock")
	lockFile, err := OpenOrCreateFile(lockPath)
	if err != nil {
		t.Fatalf("OpenOrCreateFile = %v", err)
	}
	assertExactlyRestricted(t, lockPath)
	if err := lockFile.Close(); err != nil {
		t.Fatal(err)
	}
	installPermissiveInheritedDACL(t, lockPath)
	reopenedLock, err := OpenOrCreateFile(lockPath)
	if err != nil {
		t.Fatalf("OpenOrCreateFile(existing) = %v", err)
	}
	defer reopenedLock.Close()
	assertExactlyRestricted(t, lockPath)
	if _, err := reopenedLock.Write([]byte{0}); err != nil {
		t.Fatalf("existing private handle is not writable: %v", err)
	}
}

func TestCreateTempCleansAnEmptyCandidateWhenPrivateCreationFails(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("post-create validation failed")
	create := func(path string) (*os.File, bool, error) {
		file, created, err := createPrivateFile(path)
		if err != nil {
			return file, created, err
		}
		return file, created, want
	}

	if _, err := createTempWith(directory, ".secret-", bytes.NewReader(make([]byte, 16)), create); !errors.Is(err, want) {
		t.Fatalf("createTempWith = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed private creation left entries: %#v", entries)
	}
}

func TestCreateTempDoesNotRemoveACandidateItDidNotCreate(t *testing.T) {
	directory := t.TempDir()
	candidate := filepath.Join(directory, ".secret-"+strings.Repeat("00", tempRandomByteCount))
	if err := os.Mkdir(candidate, 0o700); err != nil {
		t.Fatal(err)
	}

	random := bytes.NewReader(make([]byte, tempRandomByteCount*tempCollisionLimit))
	if _, err := createTempWith(directory, ".secret-", random, createPrivateFile); err == nil {
		t.Fatal("createTempWith unexpectedly succeeded over an existing directory candidate")
	}
	if info, err := os.Stat(candidate); err != nil {
		t.Fatalf("unowned candidate was removed: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("unowned candidate type changed: %v", info.Mode())
	}
}

func TestCreateTempStopsBeforeCreateWhenRandomnessFails(t *testing.T) {
	want := errors.New("random source failed")
	createCalled := false
	create := func(string) (*os.File, bool, error) {
		createCalled = true
		return nil, false, nil
	}

	if _, err := createTempWith(t.TempDir(), ".secret-", errorReader{err: want}, create); !errors.Is(err, want) {
		t.Fatalf("createTempWith = %v, want %v", err, want)
	}
	if createCalled {
		t.Fatal("create was called after random generation failed")
	}
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestPrivateDirectoryValidationRejectsBroadOrRelativeTargets(t *testing.T) {
	volumeRoot := filepath.VolumeName(t.TempDir()) + string(os.PathSeparator)
	for name, path := range map[string]string{
		"empty":       "",
		"relative":    "relative\\state",
		"volume root": volumeRoot,
		"UNC root":    `\\server\share\`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePrivateDirectoryPath(path); !errors.Is(err, os.ErrInvalid) {
				t.Fatalf("validatePrivateDirectoryPath(%q) = %v, want os.ErrInvalid", path, err)
			}
		})
	}
}

func TestRestrictRejectsWrongTypeAndFinalReparsePoint(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestrictFile(directory); !errors.Is(err, ErrUnexpectedType) {
		t.Fatalf("RestrictFile(directory) = %v, want ErrUnexpectedType", err)
	}
	if err := RestrictDirectory(file); !errors.Is(err, ErrUnexpectedType) {
		t.Fatalf("RestrictDirectory(file) = %v, want ErrUnexpectedType", err)
	}

	targetDirectory := filepath.Join(directory, "target-directory")
	if err := os.Mkdir(targetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(directory, "junction")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, targetDirectory).CombinedOutput(); err != nil {
		t.Fatalf("create junction fixture: %v: %s", err, output)
	}
	if err := RestrictDirectory(junction); !errors.Is(err, ErrReparsePoint) {
		t.Fatalf("RestrictDirectory(reparse) = %v, want ErrReparsePoint", err)
	}
}

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
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("GetNamedSecurityInfo(%q) = %v", path, err)
	}
	defer runtime.KeepAlive(descriptor)
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatal(err)
	}
	if !owner.Equals(testCurrentUserSID(t)) {
		t.Fatalf("%q owner is not the current user", path)
	}

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

//go:build windows

package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

func TestOSFileSystemTightensExistingWindowsPrivateState(t *testing.T) {
	root := t.TempDir()
	originalRoot := snapshotDACL(t, root)
	t.Cleanup(func() { restoreDACL(t, root, originalRoot) })
	installPermissiveInheritedDACL(t, root)

	stateDirectory := filepath.Join(root, "state")
	if err := os.Mkdir(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := OSFileSystem{}
	if err := fileSystem.MkdirAll(stateDirectory, DirectoryPermission); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", stateDirectory, err)
	}
	assertWindowsPrivatePath(t, stateDirectory)

	temporaryPath, err := fileSystem.WriteTemp(stateDirectory, ".sshc-", FilePermission, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivatePath(t, temporaryPath)

	finalPath := filepath.Join(stateDirectory, "vault")
	if err := fileSystem.Rename(temporaryPath, finalPath); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivatePath(t, finalPath)
	contents, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "sealed" {
		t.Fatalf("final contents = %q, want %q", contents, "sealed")
	}
}

func TestOSFileSystemWriteTempRemovesEmptyFileWhenDACLRestrictionFails(t *testing.T) {
	directory := t.TempDir()
	want := errors.New("DACL restriction failed")
	original := restrictPrivatePathImpl
	restrictPrivatePathImpl = func(path string, directory bool) error {
		if directory {
			return original(path, directory)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read temp before forced DACL failure: %v", err)
		}
		if len(contents) != 0 {
			t.Fatalf("temp contained %q before DACL failure", contents)
		}
		return want
	}
	t.Cleanup(func() { restrictPrivatePathImpl = original })

	if _, err := (OSFileSystem{}).WriteTemp(directory, ".sshc-", FilePermission, []byte("secret")); !errors.Is(err, want) {
		t.Fatalf("WriteTemp error = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("DACL failure left temporary entries: %#v", entries)
	}
}

func TestOSFileSystemReadFileRefusesWindowsReparsePoints(t *testing.T) {
	directory := t.TempDir()
	fileTarget := filepath.Join(directory, "target")
	if err := os.WriteFile(fileTarget, []byte("Host example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directoryTarget := filepath.Join(directory, "target-directory")
	if err := os.Mkdir(directoryTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	insideDirectoryTarget := filepath.Join(directoryTarget, "config")
	if err := os.WriteFile(insideDirectoryTarget, []byte("Host inside-directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("file symlink", func(t *testing.T) {
		link := filepath.Join(directory, "file-symlink")
		if err := os.Symlink(fileTarget, link); err != nil {
			t.Skipf("Windows symlink creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
	})
	t.Run("directory symlink", func(t *testing.T) {
		link := filepath.Join(directory, "directory-symlink")
		if err := os.Symlink(directoryTarget, link); err != nil {
			t.Skipf("Windows directory symlink creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
		assertReadFileRejectsReparsePoint(t, filepath.Join(link, "config"))
	})
	t.Run("junction", func(t *testing.T) {
		link := filepath.Join(directory, "junction")
		if err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, directoryTarget).Run(); err != nil {
			t.Skipf("Windows junction creation is unavailable: %v", err)
		}
		assertReadFileRejectsReparsePoint(t, link)
		assertReadFileRejectsReparsePoint(t, filepath.Join(link, "config"))
	})
}

func assertReadFileRejectsReparsePoint(t *testing.T, path string) {
	t.Helper()
	if _, err := (OSFileSystem{}).ReadFile(path); !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("ReadFile(%q) error = %v, want ErrSymlinkPath", path, err)
	}
}

func TestOSFileSystemReadFileAcceptsDriveCaseAndShortNames(t *testing.T) {
	directory := t.TempDir()
	if strings.HasPrefix(directory, `\\`) {
		t.Skipf("local-drive test requires a drive-letter temp directory, got %q", directory)
	}
	target := filepath.Join(directory, "Mixed Case File.txt")
	if err := os.WriteFile(target, []byte("Host case-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("drive and case", func(t *testing.T) {
		request := filepath.Join(directory, "mixed case file.txt")
		contents, err := (OSFileSystem{}).ReadFile(request)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v", request, err)
		}
		if string(contents) != "Host case-test\n" {
			t.Fatalf("contents = %q, want %q", contents, "Host case-test\n")
		}
	})
	t.Run("8.3 short name", func(t *testing.T) {
		shortPath := windowsShortPath(t, target)
		if strings.EqualFold(shortPath, target) {
			t.Skipf("8.3 short aliases are unavailable for %q", target)
		}
		contents, err := (OSFileSystem{}).ReadFile(shortPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v", shortPath, err)
		}
		if string(contents) != "Host case-test\n" {
			t.Fatalf("contents = %q, want %q", contents, "Host case-test\n")
		}
	})
}

func TestOSFileSystemReadFileAcceptsConfiguredUNCPath(t *testing.T) {
	path := os.Getenv("SSHC_WINDOWS_TEST_UNC_FILE")
	if path == "" {
		t.Skip("SSHC_WINDOWS_TEST_UNC_FILE is not configured")
	}
	if !strings.HasPrefix(path, `\\`) {
		t.Fatalf("SSHC_WINDOWS_TEST_UNC_FILE = %q, want UNC path", path)
	}
	contents, err := (OSFileSystem{}).ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", path, err)
	}
	if len(contents) == 0 {
		t.Fatal("ReadFile returned empty content for configured UNC fixture")
	}
}

func TestOSFileSystemReadFileTraversesParentWithoutReadAttributes(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "traverse-only")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "readable")
	if err := os.WriteFile(child, []byte("Host traverse-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parentDACL := snapshotDACL(t, parent)
	childDACL := snapshotDACL(t, child)
	t.Cleanup(func() {
		restoreDACL(t, child, childDACL)
		restoreDACL(t, parent, parentDACL)
	})

	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var pinner runtime.Pinner
	pinner.Pin(user.User.Sid)
	defer pinner.Unpin()
	installProtectedDACL(t, child, user.User.Sid, windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE|windows.DELETE|windows.WRITE_DAC)
	installProtectedDACL(t, parent, user.User.Sid, windows.FILE_TRAVERSE|windows.DELETE|windows.WRITE_DAC)
	assertReadAttributesDenied(t, parent)

	contents, err := (OSFileSystem{}).ReadFile(child)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", child, err)
	}
	if string(contents) != "Host traverse-test\n" {
		t.Fatalf("contents = %q, want %q", contents, "Host traverse-test\n")
	}
}

func windowsShortPath(t *testing.T, path string) string {
	t.Helper()
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetShortPathName(pathUTF16, &buffer[0], uint32(len(buffer)))
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatalf("GetShortPathName(%q) returned no path", path)
		}
		if n < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer)
		}
		buffer = make([]uint16, n+1)
	}
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
	return daclSnapshot{
		descriptor: descriptor,
		dacl:       dacl,
		protected:  control&windows.SE_DACL_PROTECTED != 0,
	}
}

func restoreDACL(t *testing.T, path string, snapshot daclSnapshot) {
	t.Helper()
	securityInformation := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	if snapshot.protected {
		securityInformation |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		securityInformation |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, securityInformation, nil, nil, snapshot.dacl, nil); err != nil {
		t.Errorf("restore DACL for %q: %v", path, err)
	}
	runtime.KeepAlive(snapshot.descriptor)
}

func installProtectedDACL(t *testing.T, path string, sid *windows.SID, permissions windows.ACCESS_MASK) {
	t.Helper()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: permissions,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(acl)
}

func installPermissiveInheritedDACL(t *testing.T, path string) {
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
	descriptor, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GRGX;;;BU)")
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
	runtime.KeepAlive(user)
}

func assertWindowsPrivatePath(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("%q has a permissive or inherited DACL", path)
	}
}

func assertReadAttributesDenied(t *testing.T, path string) {
	t.Helper()
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("fixture unexpectedly grants FILE_READ_ATTRIBUTES on the parent")
	}
	if err != windows.ERROR_ACCESS_DENIED {
		t.Fatalf("CreateFile(FILE_READ_ATTRIBUTES) error = %v, want ERROR_ACCESS_DENIED", err)
	}
}

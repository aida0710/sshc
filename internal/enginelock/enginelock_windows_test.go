//go:build windows

package enginelock

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

// ロックファイルと state ディレクトリは、作成した時点で現在のユーザー所有かつ
// 保護 DACL でなければならない。あとから締めるのでは、その隙間に他人が開ける。
func TestAcquireLeavesWindowsLockStatePrivateAndNonReparse(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire = %v", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			t.Fatal(releaseErr)
		}
	}()

	for _, target := range []string{filepath.Dir(path), path} {
		restricted, checkErr := windowsacl.IsRestrictedToCurrentUser(target)
		if checkErr != nil {
			t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", target, checkErr)
		}
		if !restricted {
			t.Fatalf("%q is not restricted to the current user", target)
		}
		info, statErr := os.Lstat(target)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			t.Fatalf("%q is a reparse point", target)
		}
	}
}

// 他人が所有するロックファイルは、ロックを取る前に拒否しなければならない。
// 受け入れてしまえば、所有権の直列化を他人の書ける状態に委ねることになる。
func TestAcquireRefusesAForeignOwnerLockFileBeforeLocking(t *testing.T) {
	path := lockPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	installForeignOwnerExactDACL(t, path)

	second, err := Acquire(path)
	if !errors.Is(err, windowsacl.ErrUnexpectedOwner) {
		t.Fatalf("Acquire on a foreign-owner lock = %v, want ErrUnexpectedOwner", err)
	}
	if second != nil {
		t.Fatal("a refused Acquire returned a release function")
	}

	// 拒否がロックより前で起きた証拠。ロックを取っていれば、別プロセスは
	// busy を受け取るはずである。
	line, code := lockInSeparateProcess(t, path)
	if code != helperExitFailed || !strings.Contains(line, "unexpected owner") {
		t.Fatalf("separate process = %q, exit %d; want the same owner refusal", line, code)
	}
}

func TestAcquireRefusesAJunctionedStateDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "state")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create privilege-free junction fixture: %v: %s", err, output)
	}

	release, err := Acquire(filepath.Join(junction, "engine.lock"))
	if !errors.Is(err, windowsacl.ErrReparsePoint) {
		t.Fatalf("Acquire through a junctioned state directory = %v, want ErrReparsePoint", err)
	}
	if release != nil {
		t.Fatal("a refused Acquire returned a release function")
	}
}

// installForeignOwnerExactDACL は、現在のトークンが持つ別の owner SID を使って、
// 「自分では所有していないが自分は読み書きできる」状態を作る。管理者権限は要らない。
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

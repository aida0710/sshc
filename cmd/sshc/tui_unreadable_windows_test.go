//go:build windows

package main

import (
	"os"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

// denyConfigRead は、いまの利用者から ~/.ssh/config の読み取りを取り上げる。
//
// mode 0000 では読めなくならない。Windows の Chmod が写すのは所有者の書き込み
// ビットだけで、それは FILE_ATTRIBUTE_READONLY にしかならないからである。読めない
// という状態を作れるのは DACL だけであり、ユーザー本人を指定する拒否 ACE ひとつだけの
// protected DACL がその最小の形である。
//
// 読めるように戻してから t.TempDir の後片付けへ渡す。所有者は DACL が何であろうと
// READ_CONTROL と WRITE_DAC を失わないので、この復元は必ずできる。
func denyConfigRead(t *testing.T, path string) {
	t.Helper()
	userSID := fixtureCurrentUserSID(t)
	t.Cleanup(func() {
		if err := setFixtureDACL(path, "D:P(A;;FA;;;"+userSID.String()+")"); err != nil {
			t.Errorf("restore a readable DACL on %q: %v", path, err)
		}
	})
	if err := setFixtureDACL(path, "D:P(D;;FA;;;"+userSID.String()+")"); err != nil {
		t.Fatalf("deny read on %q: %v", path, err)
	}
}

func setFixtureDACL(path, descriptorText string) error {
	descriptor, err := windows.SecurityDescriptorFromString(descriptorText)
	if err != nil {
		return err
	}
	defer runtime.KeepAlive(descriptor)
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return os.ErrInvalid
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}

func fixtureCurrentUserSID(t *testing.T) *windows.SID {
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

//go:build windows

// Package acltest builds the Windows security fixtures that the private-state
// tests need and that no ordinary API call can produce.
package acltest

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// InstallForeignOwner は、path の所有者を、この token のものではない SID にする。
// DACL はこちらに全権を残すので、テストは後始末できる。
//
// **この token が所有者に指定できる SID は、どれも「自分のもの」である。**
// TokenOwner と利用者本人、そして SE_GROUP_OWNER の付いたグループがそれで、
// 昇格した環境ではその集合が {利用者, Administrators} に一致してしまう——
// 所有者検査が受け入れるのと同じ集合である。その外側の SID を刻むには
// SeRestorePrivilege が要る。持てない環境ではこの fixture は作れないので、
// 黙って通すのではなく、作れなかったことを述べて skip する。
func InstallForeignOwner(t *testing.T, path string) {
	t.Helper()
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	foreignOwner, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
	if err != nil {
		t.Fatal(err)
	}
	if foreignOwner.Equals(user.User.Sid) {
		t.Fatalf("the foreign-owner fixture SID %s is this token's own user", foreignOwner)
	}

	restore, err := adjustRestorePrivilege(token, windows.SE_PRIVILEGE_ENABLED)
	if err != nil {
		t.Skipf("SeRestorePrivilege is unavailable, so a foreign owner cannot be installed: %v", err)
	}
	defer func() {
		if _, err := adjustRestorePrivilege(token, 0); err != nil {
			t.Errorf("restore SeRestorePrivilege: %v", err)
		}
		runtime.KeepAlive(restore)
	}()

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
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(user)
	switch {
	case errors.Is(err, windows.ERROR_INVALID_OWNER), errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD):
		t.Skipf("this token may not assign %s as an owner: %v", foreignOwner, err)
	case err != nil:
		t.Fatalf("install foreign-owner fixture: %v", err)
	}
}

// adjustRestorePrivilege は SeRestorePrivilege を切り替える。AdjustTokenPrivileges
// は権限を持たなくても成功を返し、ERROR_NOT_ALL_ASSIGNED を last error に置くだけ
// なので、そこまで確かめないと「有効にできた」とは言えない。
func adjustRestorePrivilege(token windows.Token, attributes uint32) (windows.Tokenprivileges, error) {
	var luid windows.LUID
	name, err := windows.UTF16PtrFromString("SeRestorePrivilege")
	if err != nil {
		return windows.Tokenprivileges{}, err
	}
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return windows.Tokenprivileges{}, err
	}
	state := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: attributes}},
	}
	var previous windows.Tokenprivileges
	var length uint32
	if err := windows.AdjustTokenPrivileges(
		token,
		false,
		&state,
		uint32(unsafe.Sizeof(previous)),
		&previous,
		&length,
	); err != nil {
		return windows.Tokenprivileges{}, err
	}
	if last := windows.GetLastError(); errors.Is(last, windows.ERROR_NOT_ALL_ASSIGNED) {
		return windows.Tokenprivileges{}, last
	}
	return previous, nil
}

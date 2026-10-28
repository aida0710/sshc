//go:build windows

package windowsacl

import (
	"runtime"

	"golang.org/x/sys/windows"
)

// この木が非公開のもの（状態のファイル、鍵、ssh-agent の pipe）に触れてよいとする相手は、
// このユーザー本人・SYSTEM・Administrators の3者である。SYSTEM と Administrators は、
// もともとこのユーザーのファイルを読めるので、外しても守りは増えない。

// CurrentUserSID は、このユーザー本人の SID を返す。
//
// DACL には現在のユーザー SID を使用する。所有者は ownedByThisToken で別に判定する。
// 所有者 SID を使用すると、昇格した環境で Administrators の ACE が重複する。
func CurrentUserSID() (*windows.SID, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid, err := user.User.Sid.Copy()
	runtime.KeepAlive(user)
	return sid, err
}

// IsTrustedPrincipal は、sid が3者（currentUser・SYSTEM・Administrators）のどれかかを答える。
func IsTrustedPrincipal(sid, currentUser *windows.SID) bool {
	if sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return true
	}
	return currentUser != nil && sid.Equals(currentUser)
}

// trustedPrincipals は、3者の SID を user・SYSTEM・Administrators の順で返す。
func trustedPrincipals(user *windows.SID) ([]*windows.SID, error) {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}
	return []*windows.SID{user, system, administrators}, nil
}

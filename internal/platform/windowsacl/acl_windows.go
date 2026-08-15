//go:build windows

// Package windowsacl は、秘密を置く state に Windows のアクセス制御を適用する。
package windowsacl

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fullAccess = windows.ACCESS_MASK(0x001f01ff)

// RestrictFile は、親から継承した緩い ACL を秘密ファイルへ残さない。
func RestrictFile(path string) error { return restrict(path) }

// RestrictDirectory は、子の作成前に state directory の継承 ACL を止める。
func RestrictDirectory(path string) error { return restrict(path) }

func restrict(path string) error {
	userSID, err := currentUserSID()
	if err != nil {
		return err
	}
	userSIDText := userSID.String()
	if userSIDText == "" {
		return windows.ERROR_INVALID_SID
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + userSIDText + ")(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return windows.ERROR_INVALID_ACL
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(userSID)
	return err
}

// IsRestrictedToCurrentUser は、mode や過去の Restrict 呼出しを信頼せず、DACL の
// 構造そのものから秘密を読む許可が 3 主体だけかを検証する。
func IsRestrictedToCurrentUser(path string) (bool, error) {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	defer runtime.KeepAlive(descriptor)

	control, _, err := descriptor.Control()
	if err != nil {
		return false, err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return false, nil
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, err
	}
	if dacl == nil || dacl.AceCount != 3 {
		return false, nil
	}

	userSID, err := currentUserSID()
	if err != nil {
		return false, err
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return false, err
	}
	administratorsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}
	expected := []*windows.SID{userSID, systemSID, administratorsSID}
	seen := make([]bool, len(expected))

	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return false, err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != fullAccess {
			return false, nil
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		matched := -1
		for expectedIndex, expectedSID := range expected {
			if !seen[expectedIndex] && aceSID.Equals(expectedSID) {
				matched = expectedIndex
				break
			}
		}
		if matched == -1 {
			return false, nil
		}
		seen[matched] = true
	}
	for _, matched := range seen {
		if !matched {
			return false, nil
		}
	}
	runtime.KeepAlive(userSID)
	runtime.KeepAlive(systemSID)
	runtime.KeepAlive(administratorsSID)
	return true, nil
}

func currentUserSID() (*windows.SID, error) {
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

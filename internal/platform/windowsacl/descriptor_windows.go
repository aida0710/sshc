//go:build windows

package windowsacl

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fullAccess は FILE_ALL_ACCESS（SDDL の FA）である。privateSecurityDescriptor が
// 3 つの ACE に与える権限で、読み返して確かめる側はこの値だけを受け入れる。
const fullAccess = windows.ACCESS_MASK(0x001f01ff)

func isHandleRestricted(handle windows.Handle, directory bool, userSID *windows.SID) (bool, error) {
	if err := validateHandleType(handle, directory); err != nil {
		return false, err
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	if descriptor == nil {
		return false, nil
	}
	defer runtime.KeepAlive(descriptor)
	return isDescriptorRestricted(descriptor, userSID, directory)
}

func isDescriptorRestricted(descriptor *windows.SECURITY_DESCRIPTOR, userSID *windows.SID, directory bool) (bool, error) {
	if descriptor == nil || userSID == nil {
		return false, nil
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return false, err
	}
	if mine, err := ownedByThisToken(owner); err != nil {
		return false, err
	} else if !mine {
		return false, nil
	}
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

	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return false, err
	}
	administratorsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}
	expected := []*windows.SID{userSID, systemSID, administratorsSID}
	wantFlags := privateAceFlags(directory)
	seen := make([]bool, len(expected))
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return false, err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != wantFlags || ace.Mask != fullAccess {
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

func newPrivateSecurityDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, *windows.SID, error) {
	userSID, err := currentUserSID()
	if err != nil {
		return nil, nil, err
	}
	descriptor, err := privateSecurityDescriptor(userSID, directory)
	if err != nil {
		return nil, nil, err
	}
	return descriptor, userSID, nil
}

// privateSecurityDescriptor は、この三者だけが触れる保護された記述子を作る。
//
// ディレクトリの ACE は継承させる。Windows で保護された DACL を付けると、
// その配下で既に存在していたものが親から受け継いでいた ACE は、その場で剥がされる。
// 継承しない ACE で締めると、締めた瞬間に中身が空の DACL になり、作ったユーザー本人も
// 開けなくなる。既存の state を締め直す道が、そこで途切れる。継承する ACE なら
// 剥がされた分がこの三者に置き換わるので、まだ刻んでいない配下も同じ範囲に収まる。
// 自分で作るものは常に P 付きの明示 ACE を持つので、継承がそれを緩めることはない。
func privateSecurityDescriptor(userSID *windows.SID, directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	userSIDText := userSID.String()
	if userSIDText == "" {
		return nil, windows.ERROR_INVALID_SID
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	entry := func(sid string) string { return "(A;" + flags + ";FA;;;" + sid + ")" }
	return windows.SecurityDescriptorFromString(
		"O:" + userSIDText + "D:P" + entry(userSIDText) + entry("SY") + entry("BA"),
	)
}

// privateAceFlags は、上の記述子が実際に刻む ACE フラグである。読み返して
// 一致を確かめる側は、これと同じ値だけを受け入れる。
func privateAceFlags(directory bool) uint8 {
	if directory {
		return windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	return 0
}

// currentUserSID は、このユーザー本人の SID を返す。
//
// DACL には現在のユーザー SID を使用する。所有者は ownedByThisToken で別に判定する。
// 所有者 SID を使用すると、昇格した環境で Administrators の ACE が重複する。
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

// tokenOwnerInformation は TOKEN_OWNER である。SID へのポインタ一本しか無い。
type tokenOwnerInformation struct {
	Owner *windows.SID
}

// tokenOwnerSID は、TOKEN_OWNER を読む。
//
// x/sys はこの class の getter を公開していないので、GetTokenInformation を
// 直接使う。返る構造体は SID への一本のポインタである。
func tokenOwnerSID(token windows.Token) (*windows.SID, error) {
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size)
	if err != nil && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, err
	}
	if size == 0 {
		return nil, windows.ERROR_INVALID_SID
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenOwner, &buffer[0], size, &size); err != nil {
		return nil, err
	}
	owner := (*tokenOwnerInformation)(unsafe.Pointer(&buffer[0])).Owner
	if owner == nil {
		return nil, windows.ERROR_INVALID_SID
	}
	sid, err := owner.Copy()
	runtime.KeepAlive(buffer)
	return sid, err
}

// ownedByThisToken は、その所有者が「自分のもの」と言えるかを判断する。
//
// この token の所有者そのものか、そのユーザー本人であればよい。昇格していない
// ときに自分で作ったものと、昇格して作ったものは、どちらも同じユーザーのものである。
func ownedByThisToken(owner *windows.SID) (bool, error) {
	if owner == nil {
		return false, nil
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false, err
	}
	defer token.Close()
	tokenOwner, err := tokenOwnerSID(token)
	if err != nil {
		return false, err
	}
	if owner.Equals(tokenOwner) {
		return true, nil
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return false, err
	}
	matched := owner.Equals(user.User.Sid)
	runtime.KeepAlive(user)
	return matched, nil
}

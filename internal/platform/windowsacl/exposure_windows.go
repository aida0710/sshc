//go:build windows

package windowsacl

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// readRights は、「その鍵の中身を読める」に当たるビットだけである。
//
// FILE_ALL_ACCESS を混ぜてはならない。あれはファイル固有の全ビット
// （0x1ff）を含むので、OR で足すと書き込みも追記も属性の読みも「読める」に
// なる。実際、書き込みだけを与えた相手が露出として報告された。
//
// 足す必要も無い。FILE_ALL_ACCESS を持つ相手は FILE_READ_DATA も持っている
// ので、下の一つ目で捕まる。GENERIC_READ と GENERIC_ALL は、ファイル固有の
// ビットへ写像される前の形のまま ACE に載ることがあるので、別に見る。
const readRights = windows.FILE_READ_DATA |
	windows.GENERIC_READ |
	windows.GENERIC_ALL

// ReadableByOthers は、この利用者本人・SYSTEM・Administrators 以外の誰かが、
// その道の中身を読めるか、読めるようにできるかを返す。
//
// これが Windows で判定できる唯一の問いである。mode ビットには誰が読める
// かが入っていない。Go は通常ファイルに 0666 を合成して返すだけで、それを
// Unix と同じ式で見れば秘密鍵は必ず「危険」になる。誰が読めるかを決めているのは
// 所有者と DACL であり、だからここは記述子を読む。判定の規則は readableByOthers
// にある。
func ReadableByOthers(path string) (bool, error) {
	file, err := openFileNoReparse(path, windows.READ_CONTROL)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return false, err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return false, err
	}
	security, err := keySecurityOf(descriptor, owner)
	if err != nil {
		return false, err
	}
	notOthers, err := principalsNotOthers(owner)
	if err != nil {
		return false, err
	}
	exposed, err := readableByOthers(security, notOthers)
	if err != nil {
		return exposed, fmt.Errorf("%s: %w", path, err)
	}
	return exposed, nil
}

// keySecurityOf は、記述子の所有者と DACL を、判定に要る形へ写す。
func keySecurityOf(descriptor *windows.SECURITY_DESCRIPTOR, owner *windows.SID) (keySecurity, error) {
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return keySecurity{}, err
	}
	security := keySecurity{hasDACL: dacl != nil}
	if owner != nil {
		security.owner = owner.String()
	}
	if dacl == nil {
		return security, nil
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var header *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &header); err != nil {
			return keySecurity{}, err
		}
		security.entries = append(security.entries, accessEntryOf(header))
	}
	return security, nil
}

// accessEntryOf は ACE ひとつを写す。相手の SID を読むのは許可の ACE だけである。
// object ACE などは SID の位置が違い、拒否の ACE は判定に相手を使わない。
func accessEntryOf(header *windows.ACCESS_ALLOWED_ACE) accessEntry {
	entry := accessEntry{aceType: header.Header.AceType, grantsRead: header.Mask&readRights != 0}
	switch header.Header.AceType {
	case windows.ACCESS_ALLOWED_ACE_TYPE:
		entry.kind = accessAllowed
		entry.trustee = (*windows.SID)(unsafe.Pointer(&header.SidStart)).String()
	case windows.ACCESS_DENIED_ACE_TYPE:
		entry.kind = accessDenied
	default:
		entry.kind = accessUnreadable
	}
	return entry
}

// principalsNotOthers は、鍵の所有者や読み手として現れても「ほかの誰か」に
// 数えない SID を返す。
//
// 記述子の所有者だけでは足りない。昇格したトークンが作ったファイルの
// 所有者は、その利用者ではなく Administrators になる。そこで所有者だけを
// 見ると、鍵を実際に持っているユーザー本人が「別のユーザー」に分類される。
// 普通に閉じている鍵が、全部危険と報告されることになる。
//
// 実機で確かめた: 昇格した SSH セッションが書いたファイルの所有者は
// S-1-5-32-544 で、DACL が読みを与えている相手はその利用者の SID だった。
//
// 所有者を入れるのは、それがこのトークンのものと言えるときだけである。
// 書き込みの側（restrictNativeHandle）と同じく ownedByThisToken で判断する。
func principalsNotOthers(owner *windows.SID) ([]string, error) {
	me, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}
	notOthers := []string{me.String(), system.String(), administrators.String()}
	mine, err := ownedByThisToken(owner)
	if err != nil {
		return nil, err
	}
	if mine {
		notOthers = append(notOthers, owner.String())
	}
	return notOthers, nil
}

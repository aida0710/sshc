//go:build windows

package windowsacl

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// readRights は、「その鍵を読める」に当たる権利をまとめたものである。
//
// `FILE_READ_DATA` だけでは足りない。`GENERIC_READ` と `GENERIC_ALL` は
// ファイルへ写像される前の形でそのまま ACE に載ることがあり、`FILE_ALL_ACCESS`
// は当然読みを含む。**どれか一つでも立っていれば、その相手は中身を読める。**
const readRights = windows.FILE_READ_DATA |
	windows.GENERIC_READ |
	windows.GENERIC_ALL |
	// FILE_ALL_ACCESS は x/sys が名前を持たないので、その定義をそのまま書く。
	// 標準権利すべてと同期、そしてファイル固有の全ビットである。
	(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff)

// ReadableByOthers は、その道の中身を、所有者・SYSTEM・Administrators 以外の
// 誰かが読めるかを答える。
//
// **これが Windows で答えられる唯一の問いである。** mode ビットには誰が読める
// かが入っていない——Go は通常ファイルに 0666 を合成して返すだけで、それを
// Unix と同じ式で見れば秘密鍵は必ず「危険」になる。誰が読めるかを決めているのは
// DACL であり、だからここは DACL を歩く。
//
// **拒否 (deny) は数えない。** 許可と拒否の効き方は ACE の並び順で決まり、
// 順序の壊れた DACL では許可が先に効く。そこで拒否を引き算すると、実際には
// 読める鍵を「安全」と報告しうる——**間違える方向としてそれが最も悪い。**
// 読ませない意図の deny があるのに警告が出るのは、その逆よりずっと軽い。
//
// **読めない形は、安全とみなさない。** 解釈できない種類の ACE に出会ったら、
// 閉じていることを確かめられなかったのだから、危険の側へ倒す。
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
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, err
	}
	// **DACL が無いことは、誰でも読めることである。** 空の DACL（ACE が
	// ひとつも無い）とは違う——あちらは誰も読めない。
	if dacl == nil {
		return true, nil
	}

	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return false, err
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}

	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var header *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &header); err != nil {
			return false, err
		}
		switch header.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			// 上の理由により数えない。
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if header.Mask&readRights == 0 {
				continue
			}
			trustee := (*windows.SID)(unsafe.Pointer(&header.SidStart))
			if trustee.Equals(owner) || trustee.Equals(system) || trustee.Equals(administrators) {
				continue
			}
			return true, nil
		default:
			// object ACE などは、ここでは読み方を持たない。ファイルの DACL に
			// 現れることはまず無いが、現れたなら**閉じていると言える根拠が
			// 無い**ので、そう答える。
			return true, fmt.Errorf("%s carries an access entry of type %d that sshc does not read",
				path, header.Header.AceType)
		}
	}
	return false, nil
}

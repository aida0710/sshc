//go:build windows

// Package windowsacl は sshc の非公開状態に Windows の所有権と DACL の規則を適用する。
package windowsacl

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ErrUnexpectedOwner = errors.New("private Windows object has an unexpected owner")
	ErrUnexpectedType  = errors.New("private Windows object has an unexpected type")
	ErrReparsePoint    = errors.New("private Windows object is a reparse point")
	ErrInvalidACL      = errors.New("private Windows object does not have the required ACL")
)

// RestrictDirectory は末尾の reparse point を追わずにディレクトリを開き、その
// ハンドルでポリシーを適用する。
func RestrictDirectory(path string) error {
	if err := validatePrivateDirectoryPath(path); err != nil {
		return err
	}
	file, err := openObjectToRestrict(path, directoryObject)
	if err != nil {
		return err
	}
	defer file.Close()
	return restrictDirectoryHandle(file)
}

func restrictFileHandle(file *os.File) error {
	return restrictHandle(file, false)
}

// RestrictFileHandle は現在ユーザーの所有権、通常ファイルであること、末尾の reparse
// 状態を検査し、渡されたハンドルで非公開 DACL を適用して再読込する。
func RestrictFileHandle(file *os.File) error {
	return restrictFileHandle(file)
}

func restrictDirectoryHandle(file *os.File) error {
	return restrictHandle(file, true)
}

func restrictHandle(file *os.File, directory bool) error {
	if file == nil {
		return os.ErrInvalid
	}
	handle := windows.Handle(file.Fd())
	err := restrictNativeHandle(handle, directory)
	runtime.KeepAlive(file)
	return err
}

// RestrictDirectoryNativeHandle は所有権を移さず、検証済みの native directory
// handle に非公開 DACL を適用する。
func RestrictDirectoryNativeHandle(handle windows.Handle) error {
	if handle == 0 || handle == windows.InvalidHandle {
		return os.ErrInvalid
	}
	return restrictNativeHandle(handle, true)
}

func restrictNativeHandle(handle windows.Handle, directory bool) error {
	if err := validateHandleType(handle, directory); err != nil {
		return err
	}
	userSID, err := currentUserSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	if descriptor == nil {
		return ErrInvalidACL
	}
	defer runtime.KeepAlive(descriptor)
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	if mine, err := ownedByThisToken(owner); err != nil {
		return err
	} else if !mine {
		return ErrUnexpectedOwner
	}

	privateDescriptor, err := privateSecurityDescriptor(userSID, directory)
	if err != nil {
		return err
	}
	dacl, _, err := privateDescriptor.DACL()
	if err != nil || dacl == nil {
		if err != nil {
			return err
		}
		return ErrInvalidACL
	}
	if err := windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return err
	}
	runtime.KeepAlive(privateDescriptor)

	restricted, err := isHandleRestricted(handle, directory, userSID)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(userSID)
	if err != nil {
		return err
	}
	if !restricted {
		return ErrInvalidACL
	}
	return nil
}

// IsRestrictedToCurrentUser は reparse でないファイルまたはディレクトリを開いて検査する。
// 以前の Restrict 呼び出しや POSIX mode bit は根拠にしない。
func IsRestrictedToCurrentUser(path string) (bool, error) {
	if err := ValidatePrivatePath(path); err != nil {
		return false, err
	}
	file, err := openObjectToInspect(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	handle := windows.Handle(file.Fd())
	if err := validateHandleTypeAny(handle); err != nil {
		return false, err
	}
	userSID, err := currentUserSID()
	if err != nil {
		return false, err
	}
	info := windows.ByHandleFileInformation{}
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return false, err
	}
	directory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	return isHandleRestricted(handle, directory, userSID)
}

// OpenAuthenticatedFile は各パス要素を reparse を追わずに開き、最終ハンドルで所有者と
// 保護 DACL の検証に成功した後、その通常ファイルのハンドルを返す。後で同じオブジェクトを
// 削除できるよう、DELETE 権限は最初に要求する。
func OpenAuthenticatedFile(path string) (*os.File, error) {
	return openAuthenticatedFile(path, authenticatedFileReadAccess|windows.DELETE)
}

// OpenAuthenticatedFileForRead は読み取り専用版である。非公開状態を一定量読むだけの
// 呼び出しでは DELETE 権限を要求しない。
func OpenAuthenticatedFileForRead(path string) (*os.File, error) {
	return openAuthenticatedFile(path, authenticatedFileReadAccess)
}

// authenticatedFileReadAccess は、通常ファイルの所有者と DACL を検証してから読むのに
// 要る権限である。
const authenticatedFileReadAccess = uint32(windows.FILE_READ_DATA | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL)

func openAuthenticatedFile(path string, access uint32) (*os.File, error) {
	file, err := openFileNoReparse(path, access)
	if err != nil {
		return nil, err
	}
	userSID, err := currentUserSID()
	if err == nil {
		err = authenticateHandle(windows.Handle(file.Fd()), false, userSID)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func authenticateHandle(handle windows.Handle, directory bool, userSID *windows.SID) error {
	if err := validateHandleType(handle, directory); err != nil {
		return err
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	if descriptor == nil {
		return ErrInvalidACL
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	if mine, err := ownedByThisToken(owner); err != nil {
		return err
	} else if !mine {
		return ErrUnexpectedOwner
	}
	restricted, err := isDescriptorRestricted(descriptor, userSID, directory)
	if err != nil {
		return err
	}
	if !restricted {
		return ErrInvalidACL
	}
	return nil
}

// DeleteFileHandle は開いている通常ファイルを削除対象にする。削除を完了するには
// 呼び出し側がファイルを閉じる必要がある。
func DeleteFileHandle(file *os.File) error {
	if file == nil {
		return os.ErrInvalid
	}
	handle := windows.Handle(file.Fd())
	if err := validateHandleType(handle, false); err != nil {
		return err
	}
	return markFileForDeletion(handle)
}

type fileDispositionInfo struct {
	DeleteFile bool
}

func markFileForDeletion(handle windows.Handle) error {
	information := fileDispositionInfo{DeleteFile: true}
	return windows.SetFileInformationByHandle(
		handle,
		windows.FileDispositionInfo,
		(*byte)(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	)
}

// objectKind は、開いたハンドルが指していなければならない種類である。
type objectKind int

const (
	// anyObject は種類を問わない。所有者と DACL を読んで確かめるだけのときに使う。
	anyObject objectKind = iota
	fileObject
	directoryObject
)

// objectInspectAccess は、所有者と DACL を読むのに要る権限である。
const objectInspectAccess = uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)

// openObjectToRestrict は、DACL を書き換えるために、kind の種類のオブジェクトを開く。
func openObjectToRestrict(path string, kind objectKind) (*os.File, error) {
	return openObjectWithAccess(path, objectInspectAccess|windows.WRITE_DAC, kind)
}

// openObjectToInspect は、所有者と DACL を読むために、種類を問わずオブジェクトを開く。
func openObjectToInspect(path string) (*os.File, error) {
	return openObjectWithAccess(path, objectInspectAccess, anyObject)
}

func openPrivateFileForUse(path string) (*os.File, error) {
	access := uint32(
		windows.GENERIC_READ |
			windows.GENERIC_WRITE |
			windows.READ_CONTROL |
			windows.WRITE_DAC |
			windows.DELETE |
			windows.FILE_READ_ATTRIBUTES,
	)
	return openObjectWithAccess(path, access, fileObject)
}

func openObjectWithAccess(path string, access uint32, kind objectKind) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS)
	handle, err := windows.CreateFile(
		pathUTF16,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, os.ErrInvalid
	}
	if err := validateHandleKind(handle, kind); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validateHandleKind(handle windows.Handle, kind objectKind) error {
	if kind == anyObject {
		return validateHandleTypeAny(handle)
	}
	return validateHandleType(handle, kind == directoryObject)
}

func validateHandleType(handle windows.Handle, directory bool) error {
	info := windows.ByHandleFileInformation{}
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrReparsePoint
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != directory {
		return ErrUnexpectedType
	}
	return nil
}

func validateHandleTypeAny(handle windows.Handle) error {
	info := windows.ByHandleFileInformation{}
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrReparsePoint
	}
	return nil
}

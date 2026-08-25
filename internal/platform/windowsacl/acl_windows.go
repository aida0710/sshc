//go:build windows

// Package windowsacl は sshc の非公開状態に Windows の所有権と DACL の規則を適用する。
package windowsacl

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fullAccess          = windows.ACCESS_MASK(0x001f01ff)
	tempRandomByteCount = 16
	tempCollisionLimit  = 128
)

var (
	ErrUnexpectedOwner = errors.New("private Windows object has an unexpected owner")
	ErrUnexpectedType  = errors.New("private Windows object has an unexpected type")
	ErrReparsePoint    = errors.New("private Windows object is a reparse point")
	ErrInvalidACL      = errors.New("private Windows object does not have the required ACL")
)

// CreateTemp は、CreateFile が返る時点で所有者と保護 DACL が有効な空ファイルを作る。
// この処理が成功する前に秘密のバイト列は書き込まれない。
func CreateTemp(directory, prefix string) (*os.File, error) {
	if err := ValidatePrivatePath(directory); err != nil {
		return nil, err
	}
	return createTempWith(directory, prefix, rand.Reader, createPrivateFile)
}

func createTempWith(directory, prefix string, random io.Reader, create func(string) (*os.File, bool, error)) (*os.File, error) {
	if prefix != filepath.Base(prefix) || strings.ContainsRune(prefix, os.PathSeparator) {
		return nil, os.ErrInvalid
	}
	for attempt := 0; attempt < tempCollisionLimit; attempt++ {
		randomBytes := make([]byte, tempRandomByteCount)
		if _, err := io.ReadFull(random, randomBytes); err != nil {
			return nil, err
		}
		path := filepath.Join(directory, prefix+hex.EncodeToString(randomBytes))
		file, created, err := create(path)
		for index := range randomBytes {
			randomBytes[index] = 0
		}
		if err == nil {
			return file, nil
		}
		if created {
			if cleanupErr := discardCreatedFile(file); cleanupErr != nil {
				return nil, errors.Join(err, fmt.Errorf("remove failed private Windows temp: %w", cleanupErr))
			}
		}
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("create private Windows temp: collision limit exceeded")
}

// OpenOrCreateFile は非公開の空ファイルをアトミックに作成する。既存の場合は、現在の
// ユーザーが所有する通常ファイルを開き、呼び出し側へ返す同じハンドルで権限を制限する。
func OpenOrCreateFile(path string) (*os.File, error) {
	if err := ValidatePrivatePath(path); err != nil {
		return nil, err
	}
	file, created, err := createPrivateFile(path)
	if err == nil {
		return file, nil
	}
	if created {
		cleanupErr := discardCreatedFile(file)
		return nil, errors.Join(err, cleanupErr)
	}
	if !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	file, err = openPrivateFileForUse(path)
	if err != nil {
		return nil, err
	}
	if err := restrictFileHandle(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// OpenPrivateDirectory opens and retains the exact private directory identity.
// Every component is resolved with OBJ_DONT_REPARSE and the returned handle is
// authenticated before it can be used as a relative-create root.
func OpenPrivateDirectory(path string) (*os.File, error) {
	if err := EnsureDirectory(path); err != nil {
		return nil, err
	}
	access := uint32(windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.WRITE_DAC)
	directory, err := openDirectoryNoReparse(path, access)
	if err != nil {
		return nil, err
	}
	if err := restrictDirectoryHandle(directory); err != nil {
		_ = directory.Close()
		return nil, err
	}
	return directory, nil
}

// OpenOrCreateFileAt creates or opens name relative to an already authenticated
// directory handle. A concurrent rename or junction replacement of the path
// used to obtain that directory therefore cannot redirect the file operation.
func OpenOrCreateFileAt(directory *os.File, name string) (*os.File, error) {
	if directory == nil || name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) {
		return nil, os.ErrInvalid
	}
	descriptor, userSID, err := newPrivateSecurityDescriptor(false)
	if err != nil {
		return nil, err
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:             uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory:      windows.Handle(directory.Fd()),
		ObjectName:         objectName,
		Attributes:         windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		SecurityDescriptor: descriptor,
	}
	var handle windows.Handle
	status := windows.IO_STATUS_BLOCK{}
	err = windows.NtCreateFile(
		&handle,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.WRITE_DAC|windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		attributes,
		&status,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN_IF,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	runtime.KeepAlive(directory)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(userSID)
	if err != nil {
		return nil, mapNoReparseError(err)
	}
	file := os.NewFile(uintptr(handle), filepath.Join(directory.Name(), name))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, os.ErrInvalid
	}
	if err := restrictFileHandle(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// EnsureDirectory は不足している各要素を非公開 descriptor で作る。既存の親は変更せず、
// 最終ディレクトリを一度だけ開いて現在ユーザーの所有権、種類、reparse 状態を検査し、
// そのハンドルで権限を制限する。
func EnsureDirectory(path string) error {
	if err := validatePrivateDirectoryPath(path); err != nil {
		return err
	}
	cleaned := filepath.Clean(path)
	if err := ensureParents(cleaned); err != nil {
		return err
	}
	err := createPrivateDirectory(cleaned)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return err
	}
	return RestrictDirectory(cleaned)
}

func validatePrivateDirectoryPath(path string) error {
	if err := ValidatePrivatePath(path); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return os.ErrInvalid
	}
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	if volume == "" {
		return os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	if strings.EqualFold(cleaned, filepath.Clean(root)) {
		return os.ErrInvalid
	}
	return nil
}

func ensureParents(path string) error {
	parent := filepath.Dir(path)
	if parent == path {
		return nil
	}
	info, err := os.Lstat(parent)
	switch {
	case err == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return ErrReparsePoint
		}
		if !info.IsDir() {
			return ErrUnexpectedType
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := ensureParents(parent); err != nil {
		return err
	}
	if err := createPrivateDirectory(parent); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return err
	}
	return RestrictDirectory(parent)
}

// RestrictFile は末尾の reparse point を追わずに最終パスを開き、そのハンドルで
// ポリシーを適用する。
func RestrictFile(path string) error {
	if err := ValidatePrivatePath(path); err != nil {
		return err
	}
	file, err := openObject(path, true, false)
	if err != nil {
		return err
	}
	defer file.Close()
	return restrictFileHandle(file)
}

// RestrictDirectory は RestrictFile のディレクトリ版である。
func RestrictDirectory(path string) error {
	if err := validatePrivateDirectoryPath(path); err != nil {
		return err
	}
	file, err := openObject(path, true, true)
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
	file, err := openObject(path, false, false)
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
	return openAuthenticatedFile(path, true)
}

// OpenAuthenticatedFileForRead は読み取り専用版である。非公開状態を一定量読むだけの
// 呼び出しでは DELETE 権限を要求しない。
func OpenAuthenticatedFileForRead(path string) (*os.File, error) {
	return openAuthenticatedFile(path, false)
}

func openAuthenticatedFile(path string, removable bool) (*os.File, error) {
	access := uint32(windows.FILE_READ_DATA | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL)
	if removable {
		access |= windows.DELETE
	}
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

func createPrivateFile(path string) (*os.File, bool, error) {
	descriptor, userSID, err := newPrivateSecurityDescriptor(false)
	if err != nil {
		return nil, false, err
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	attributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.WRITE_DAC|windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(userSID)
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		cleanupErr := errors.Join(markFileForDeletion(handle), windows.CloseHandle(handle))
		return nil, false, errors.Join(os.ErrInvalid, cleanupErr)
	}
	restricted, err := isHandleRestricted(handle, false, userSID)
	if err == nil && !restricted {
		err = ErrInvalidACL
	}
	if err != nil {
		return file, true, err
	}
	return file, true, nil
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

// discardCreatedFile は CREATE_NEW が返したハンドルだけを操作する。別のオブジェクトへ
// 置換された可能性があるパス名は削除しない。
func discardCreatedFile(file *os.File) error {
	if file == nil {
		return nil
	}
	deleteErr := markFileForDeletion(windows.Handle(file.Fd()))
	closeErr := file.Close()
	return errors.Join(deleteErr, closeErr)
}

func createPrivateDirectory(path string) error {
	descriptor, _, err := newPrivateSecurityDescriptor(true)
	if err != nil {
		return err
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	err = windows.CreateDirectory(pathUTF16, attributes)
	runtime.KeepAlive(descriptor)
	return err
}

func openObject(path string, writable, directory bool) (*os.File, error) {
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	if writable {
		access |= windows.WRITE_DAC
	}
	return openObjectWithAccess(path, access, writable, directory)
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
	return openObjectWithAccess(path, access, true, false)
}

func openObjectWithAccess(path string, access uint32, writable, directory bool) (*os.File, error) {
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
	if writable {
		if err := validateHandleType(handle, directory); err != nil {
			_ = file.Close()
			return nil, err
		}
	} else if err := validateHandleTypeAny(handle); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
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

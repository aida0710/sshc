//go:build windows

package windowsacl

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

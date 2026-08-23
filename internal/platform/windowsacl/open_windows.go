//go:build windows

package windowsacl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openFileNoReparse は開いた親を基準に、OBJ_DONT_REPARSE を指定して各子要素を解決する。
// 最終ハンドルは検査した親の連鎖へ固定されるため、junction を置換して後続の CreateFile
// を別のパスへ誘導することはできない。
func openFileNoReparse(path string, access uint32) (*os.File, error) {
	if err := ValidatePrivatePath(path); err != nil {
		return nil, err
	}
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	if !filepath.IsAbs(cleaned) || volume == "" {
		return nil, os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	relative, err := filepath.Rel(root, cleaned)
	if err != nil || filepath.IsAbs(relative) || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil, os.ErrInvalid
	}

	current, err := openNoReparseRoot(root)
	if err != nil {
		return nil, err
	}
	components := strings.Split(relative, string(os.PathSeparator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = windows.CloseHandle(current)
			return nil, os.ErrInvalid
		}
		final := index == len(components)-1
		desiredAccess := uint32(windows.FILE_TRAVERSE)
		createOptions := uint32(windows.FILE_DIRECTORY_FILE)
		if final {
			desiredAccess = access | windows.SYNCHRONIZE
			createOptions = windows.FILE_NON_DIRECTORY_FILE | windows.FILE_SYNCHRONOUS_IO_NONALERT
		}
		next, openErr := openRelativeNoReparse(current, component, desiredAccess, createOptions)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}

	file := os.NewFile(uintptr(current), cleaned)
	if file == nil {
		_ = windows.CloseHandle(current)
		return nil, os.ErrInvalid
	}
	return file, nil
}

func openNoReparseRoot(path string) (windows.Handle, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	// drive または通常の UNC share root を名前空間の基点とする。SMB サーバーには
	// OPEN_REPARSE_POINT の実装義務がないため、ここでは使用しない。このハンドルより下の
	// 各要素には引き続き OBJ_DONT_REPARSE を使用する。
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.FILE_TRAVERSE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func openRelativeNoReparse(parent windows.Handle, name string, access, createOptions uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	status := windows.IO_STATUS_BLOCK{}
	err = windows.NtCreateFile(
		&handle,
		access,
		attributes,
		&status,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		createOptions,
		0,
		0,
	)
	if err != nil {
		return 0, mapNoReparseError(err)
	}
	return handle, nil
}

// mapNoReparseError は、NtCreateFile が返す NTSTATUS を、この木の他の層が
// 知っている形に直す。
//
// NTSTATUS は fs.ErrNotExist に一致しない。「まだ無い」と「読めなかった」を
// errors.Is で分ける呼び出し側が上に何十とあり、生の NTSTATUS を返せば、まだ
// 置かれていないだけの private state が読み取り失敗として扱われる。
func mapNoReparseError(err error) error {
	if errors.Is(err, windows.STATUS_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) ||
		errors.Is(err, windows.STATUS_IO_REPARSE_TAG_NOT_HANDLED) ||
		errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) {
		return ErrReparsePoint
	}
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status.Errno()
	}
	return err
}

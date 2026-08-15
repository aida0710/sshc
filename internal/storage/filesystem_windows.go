//go:build windows

package storage

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileAttributeTagInfo は GetFileInformationByHandleEx の
// FileAttributeTagInfo 応答と同じレイアウトである。x/sys はこの構造体を公開して
// いないため、OS 呼び出しの境界でだけ定義する。
type fileAttributeTagInfo struct {
	fileAttributes uint32
	reparseTag     uint32
}

func openRegularNoFollow(path string) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, err
	}
	closeHandle := true
	defer func() {
		if closeHandle {
			_ = windows.CloseHandle(handle)
		}
	}()

	var attributes fileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&attributes)),
		uint32(unsafe.Sizeof(attributes)),
	); err != nil {
		return nil, err
	}
	if attributes.fileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attributes.reparseTag != 0 {
		return nil, ErrSymlinkPath
	}

	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return nil, os.ErrInvalid
	}
	closeHandle = false
	return file, nil
}

func replaceFile(oldPath, newPath string) error {
	oldUTF16, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newUTF16, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldUTF16, newUTF16, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func syncDirectory(path string) error {
	// Windows には Unix の directory fsync に相当する API がない。このため
	// WriteTemp の file.Sync と replaceFile の write-through 移動を永続化境界にし、
	// ここではそれ以上の永続性を装わない。
	return nil
}

func restrictPrivatePath(path string, directory bool) error {
	// DACL によるアクセス制御は Windows 固有の Task 2 で追加する。この段階では
	// os.Chmod を DACL 保護の代替として扱わない。
	return nil
}

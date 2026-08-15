//go:build windows

package storage

import (
	"os"
	"path/filepath"
	"strings"
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
	absolutePath, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return nil, err
	}
	if err := rejectReparseParents(absolutePath); err != nil {
		return nil, err
	}

	pathUTF16, err := windows.UTF16PtrFromString(absolutePath)
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
	if err := verifyHandlePath(handle, absolutePath); err != nil {
		return nil, err
	}

	file := os.NewFile(uintptr(handle), absolutePath)
	if file == nil {
		return nil, os.ErrInvalid
	}
	closeHandle = false
	return file, nil
}

func rejectReparseParents(path string) error {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	parent := filepath.Dir(path)
	relativeParent, err := filepath.Rel(root, parent)
	if err != nil || filepath.IsAbs(relativeParent) || relativeParent == ".." || strings.HasPrefix(relativeParent, ".."+string(os.PathSeparator)) {
		return os.ErrInvalid
	}

	if err := rejectReparseDirectory(root); err != nil {
		return err
	}
	if relativeParent == "." {
		return nil
	}
	current := root
	for _, component := range strings.Split(relativeParent, string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := rejectReparseDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func rejectReparseDirectory(path string) error {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return rejectReparseHandle(handle)
}

func rejectReparseHandle(handle windows.Handle) error {
	var attributes fileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&attributes)),
		uint32(unsafe.Sizeof(attributes)),
	); err != nil {
		return err
	}
	if attributes.fileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attributes.reparseTag != 0 {
		return ErrSymlinkPath
	}
	return nil
}

func cleanAbsoluteDOSPath(path string) (string, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetFullPathName(pathUTF16, uint32(len(buffer)), &buffer[0], nil)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return normalizeDOSPath(windows.UTF16ToString(buffer))
		}
		buffer = make([]uint16, n+1)
	}
}

func verifyHandlePath(handle windows.Handle, expected string) error {
	actual, err := finalDOSPath(handle)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return ErrSymlinkPath
	}
	return nil
}

func finalDOSPath(handle windows.Handle) (string, error) {
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return normalizeDOSPath(windows.UTF16ToString(buffer))
		}
		buffer = make([]uint16, n+1)
	}
}

func normalizeDOSPath(path string) (string, error) {
	const (
		verbatimPrefix    = `\\?\`
		verbatimUNCPrefix = `\\?\UNC\`
		ntPrefix          = `\??\`
		ntUNCPrefix       = `\??\UNC\`
	)
	switch {
	case strings.HasPrefix(path, verbatimUNCPrefix):
		path = `\\` + path[len(verbatimUNCPrefix):]
	case strings.HasPrefix(path, ntUNCPrefix):
		path = `\\` + path[len(ntUNCPrefix):]
	case strings.HasPrefix(path, verbatimPrefix):
		path = path[len(verbatimPrefix):]
	case strings.HasPrefix(path, ntPrefix):
		path = path[len(ntPrefix):]
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || filepath.VolumeName(path) == "" {
		return "", os.ErrInvalid
	}
	return path, nil
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

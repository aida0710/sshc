//go:build windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openRegularNoFollow(path string) (*os.File, error) {
	absolutePath, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return nil, err
	}
	parent, err := openNoReparseParent(filepath.Dir(absolutePath))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)

	fileName := filepath.Base(absolutePath)
	if fileName == "." || fileName == string(os.PathSeparator) {
		return nil, os.ErrInvalid
	}
	handle, err := openRelativeNoReparse(parent, fileName, windows.FILE_READ_DATA, false, true)
	if err != nil {
		return nil, err
	}
	closeHandle := true
	defer func() {
		if closeHandle {
			_ = windows.CloseHandle(handle)
		}
	}()

	file := os.NewFile(uintptr(handle), absolutePath)
	if file == nil {
		return nil, os.ErrInvalid
	}
	closeHandle = false
	return file, nil
}

// openNoReparseParent は parent を handle 相対でたどる。OBJ_DONT_REPARSE を各
// component の name resolution に渡すため、検査後の置換で別 tree をたどれない。
func openNoReparseParent(path string) (windows.Handle, error) {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return 0, os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	relativeParent, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relativeParent) || relativeParent == ".." || strings.HasPrefix(relativeParent, ".."+string(os.PathSeparator)) {
		return 0, os.ErrInvalid
	}

	current, err := openAbsoluteNoReparseDirectory(root)
	if err != nil {
		return 0, err
	}
	if relativeParent == "." {
		return current, nil
	}
	for _, component := range strings.Split(relativeParent, string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		next, err := openRelativeNoReparse(current, component, windows.FILE_TRAVERSE, true, false)
		if err != nil {
			_ = windows.CloseHandle(current)
			return 0, err
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return current, nil
}

// openAbsoluteNoReparseDirectory は volume/share root を開く。RootDirectory 相対
// NtCreateFile を始めるため Win32 の directory open が必要だが、root 自体は入力の
// lexical parent ではないため FILE_TRAVERSE だけを要求する。
func openAbsoluteNoReparseDirectory(path string) (windows.Handle, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
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

// openRelativeNoReparse は既に検査済みの parent handle を RootDirectory に渡す。
// これにより parent path の名前空間が後から junction 等へ置換されても、開く
// object は保持中の親 directory に固定される。
func openRelativeNoReparse(parent windows.Handle, name string, access uint32, directory, synchronous bool) (windows.Handle, error) {
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
	var status windows.IO_STATUS_BLOCK
	options := uint32(0)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	}
	if synchronous {
		access |= windows.SYNCHRONIZE
		options |= windows.FILE_SYNCHRONOUS_IO_NONALERT
	}
	err = windows.NtCreateFile(
		&handle,
		access,
		attributes,
		&status,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		options,
		0,
		0,
	)
	if err != nil {
		return 0, mapReparseError(err)
	}
	return handle, nil
}

func mapReparseError(err error) error {
	if errors.Is(err, windows.STATUS_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) ||
		errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) {
		return ErrSymlinkPath
	}
	return err
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

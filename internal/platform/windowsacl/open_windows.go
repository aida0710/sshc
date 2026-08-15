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

// openFileNoReparse resolves every descendant relative to an already-opened
// parent with OBJ_DONT_REPARSE. The final handle is therefore bound to the same
// parent chain that was checked; a junction swap cannot redirect a later
// path-based CreateFile call.
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
	// A drive/ordinary-UNC share root is the namespace anchor rather than an
	// attacker-selected descendant. Avoid OPEN_REPARSE_POINT here because SMB
	// servers need not implement it; every component below this handle still
	// uses OBJ_DONT_REPARSE.
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

func mapNoReparseError(err error) error {
	if errors.Is(err, windows.STATUS_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) ||
		errors.Is(err, windows.STATUS_IO_REPARSE_TAG_NOT_HANDLED) ||
		errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) {
		return ErrReparsePoint
	}
	return err
}

//go:build windows

package windowsacl

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openFileNoReparse(path string, access uint32) (*os.File, error) {
	return openPathNoReparse(path, access, windows.FILE_NON_DIRECTORY_FILE)
}

func openDirectoryNoReparse(path string, access uint32) (*os.File, error) {
	return openPathNoReparse(path, access, windows.FILE_DIRECTORY_FILE)
}

// openPathNoReparse は、親までを OpenDirectoryNoReparse でたどり、最後の要素を
// その親を基準に開く。kind は FILE_NON_DIRECTORY_FILE か FILE_DIRECTORY_FILE である。
func openPathNoReparse(path string, access, kind uint32) (*os.File, error) {
	if err := ValidatePrivatePath(path); err != nil {
		return nil, err
	}
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	if !filepath.IsAbs(cleaned) || volume == "" || strings.EqualFold(cleaned, volume+string(os.PathSeparator)) {
		return nil, os.ErrInvalid
	}
	parent, err := OpenDirectoryNoReparse(filepath.Dir(cleaned), windows.FILE_TRAVERSE)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	handle, err := OpenRelative(parent, filepath.Base(cleaned), RelativeOpen{
		Access:        access | windows.SYNCHRONIZE,
		Disposition:   windows.FILE_OPEN,
		CreateOptions: kind | windows.FILE_SYNCHRONOUS_IO_NONALERT,
	})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), cleaned)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, os.ErrInvalid
	}
	return file, nil
}

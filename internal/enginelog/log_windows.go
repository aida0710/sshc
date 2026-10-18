//go:build windows

package enginelog

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"sshc/internal/platform/windowsacl"
)

func openPrivateAppend(path string) (*os.File, error) {
	directory, err := windowsacl.OpenPrivateDirectory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	file, err := windowsacl.OpenOrCreateFileAt(directory, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Removing FILE_WRITE_DATA makes each inherited write append atomically,
	// including writes from two engines racing to acquire engine.lock.
	var appendHandle windows.Handle
	process := windows.CurrentProcess()
	err = windows.DuplicateHandle(process, windows.Handle(file.Fd()), process, &appendHandle,
		windows.FILE_APPEND_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, false, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(appendHandle), path), nil
}

func openPrivateRead(path string) (*os.File, error) {
	return windowsacl.OpenAuthenticatedFileForRead(path)
}

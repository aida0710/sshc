//go:build windows

package handoff

import (
	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

func defaultWriteOperations() writeOperations {
	return writeOperations{
		ensureDirectory: windowsacl.EnsureDirectory,
		createTemp:      windowsacl.CreateTemp,
		replace:         replaceHandoffFile,
		syncDirectory:   syncHandoffDirectory,
	}
}

func replaceHandoffFile(oldPath, newPath string) error {
	oldPathUTF16, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPathUTF16, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		oldPathUTF16,
		newPathUTF16,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func syncHandoffDirectory(string) error {
	// MoveFileEx with MOVEFILE_WRITE_THROUGH is the Windows durability
	// boundary. A read-only directory handle cannot be passed to
	// FlushFileBuffers, so there is intentionally no second directory Sync.
	return nil
}

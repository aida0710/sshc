//go:build windows

package handoff

import (
	"os"

	"sshc/internal/platform/windowsacl"
)

func defaultWriteOperations() writeOperations {
	return writeOperations{ensureDirectory: windowsacl.EnsureDirectory}
}

func defaultHandoffFileOperations() handoffFileOperations {
	return handoffFileOperations{
		open: windowsacl.OpenAuthenticatedFile,
		remove: func(file *os.File, _ string) error {
			if err := windowsacl.DeleteFileHandle(file); err != nil {
				_ = file.Close()
				return err
			}
			return file.Close()
		},
	}
}

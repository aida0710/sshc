//go:build windows

package handoff

import (
	"os"

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
	// Windows の永続化には MOVEFILE_WRITE_THROUGH を指定した MoveFileEx を使用する。
	// 読み取り専用ディレクトリハンドルは FlushFileBuffers へ渡せないため、追加の
	// directory Sync は行わない。
	return nil
}

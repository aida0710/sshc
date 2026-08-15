//go:build unix

package handoff

import "os"

func defaultWriteOperations() writeOperations {
	return writeOperations{
		ensureDirectory: ensureHandoffDirectory,
		createTemp:      os.CreateTemp,
		replace:         replaceHandoffFile,
		syncDirectory:   syncHandoffDirectory,
	}
}

func defaultHandoffFileOperations() handoffFileOperations {
	return handoffFileOperations{
		open: os.Open,
		remove: func(file *os.File, path string) error {
			if err := file.Close(); err != nil {
				return err
			}
			return os.Remove(path)
		},
	}
}

func ensureHandoffDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func replaceHandoffFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func syncHandoffDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

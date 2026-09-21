//go:build unix

package handoff

import "os"

func defaultWriteOperations() writeOperations {
	return writeOperations{ensureDirectory: ensureHandoffDirectory}
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

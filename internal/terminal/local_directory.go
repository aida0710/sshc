package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Matches the terminal API bound and avoids resolving unbounded input paths.
const localWorkingDirectoryLimit = 4096

var ErrInvalidLocalWorkingDirectory = errors.New("local working directory is unavailable")

// ResolveLocalWorkingDirectory resolves a directory on the engine machine.
// Its result is passed as the process working directory, never shell input.
func ResolveLocalWorkingDirectory(candidate string) (string, error) {
	if candidate == "" || len(candidate) > localWorkingDirectoryLimit || strings.ContainsRune(candidate, 0) {
		return "", ErrInvalidLocalWorkingDirectory
	}
	directory := filepath.FromSlash(candidate)
	if !filepath.IsAbs(directory) {
		return "", ErrInvalidLocalWorkingDirectory
	}
	cleaned := filepath.Clean(directory)
	if cleaned != directory && strings.TrimRight(directory, string(filepath.Separator)) != cleaned {
		return "", ErrInvalidLocalWorkingDirectory
	}
	entry, err := os.Stat(cleaned)
	if err != nil {
		return "", errors.Join(ErrInvalidLocalWorkingDirectory, err)
	}
	if !entry.IsDir() {
		return "", ErrInvalidLocalWorkingDirectory
	}
	return cleaned, nil
}

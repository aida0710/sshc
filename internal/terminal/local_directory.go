package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidLocalWorkingDirectory = errors.New("local working directory is unavailable")

// ResolveLocalWorkingDirectory は、エンジンのマシンのフォルダを確かめる。結果は process の作業フォルダとして
// 渡し、シェルへの入力にはしない。
func ResolveLocalWorkingDirectory(candidate string) (string, error) {
	if !isDirectoryText(candidate) {
		return "", ErrInvalidLocalWorkingDirectory
	}
	directory := filepath.FromSlash(candidate)
	// 検査は os.Stat の前に行う。os.Stat はネットワークのボリュームを開いてしまう。
	if !filepath.IsAbs(directory) || !onLocalDrive(directory) {
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

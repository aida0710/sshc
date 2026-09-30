package application

import (
	"errors"
	"path/filepath"
	"strings"

	"sshc/internal/platform/nativepath"
)

var ErrExternalPath = errors.New("path is outside the ssh directory")

func RelativePath(root, absolute string) (string, error) {
	if !filepath.IsAbs(absolute) {
		return "", ErrExternalPath
	}
	relative, ok := nativepath.RelativeSlash(root, absolute)
	if !ok {
		return "", ErrExternalPath
	}
	return relative, nil
}

func AbsolutePath(root, relative string) (string, error) {
	if relative == "" || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\x00") {
		return "", ErrExternalPath
	}
	joined := filepath.Join(filepath.Clean(root), filepath.FromSlash(relative))
	if _, err := RelativePath(root, joined); err != nil {
		return "", err
	}
	return joined, nil
}

package application

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrExternalPath = errors.New("path is outside the ssh directory")

func RelativePath(root, absolute string) (string, error) {
	if !filepath.IsAbs(absolute) {
		return "", ErrExternalPath
	}
	cleaned := filepath.Clean(absolute)
	if cleaned == filepath.Clean(root) {
		return "", ErrExternalPath
	}
	relative, err := filepath.Rel(filepath.Clean(root), cleaned)
	if err != nil {
		return "", ErrExternalPath
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrExternalPath
	}
	return filepath.ToSlash(relative), nil
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

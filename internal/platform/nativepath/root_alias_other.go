//go:build !windows && !darwin

package nativepath

import (
	"os"
	"path/filepath"
)

// ResolveRootAlias is an identity operation on Unix platforms without macOS
// compatibility aliases.
func ResolveRootAlias(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", os.ErrInvalid
	}
	return cleaned, nil
}

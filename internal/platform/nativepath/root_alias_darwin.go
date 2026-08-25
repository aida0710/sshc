//go:build darwin

package nativepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// macOSRootAliases are root-owned compatibility links present on a standard
// macOS installation. In particular, os.TempDir returns /var/folders/... while
// /var points to /private/var. Descriptor-relative walkers may resolve this
// fixed system boundary once; callers must keep every lower component
// O_NOFOLLOW.
var macOSRootAliases = map[string]string{
	"etc": "/private/etc",
	"tmp": "/private/tmp",
	"var": "/private/var",
}

// ResolveRootAlias canonicalizes a documented root-owned macOS compatibility
// link only when it still points to its exact platform target. It deliberately
// does not resolve any component below the filesystem root.
func ResolveRootAlias(path string) (string, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", os.ErrInvalid
	}
	relative := strings.TrimPrefix(path, string(filepath.Separator))
	components := strings.Split(relative, string(filepath.Separator))
	if len(components) == 0 {
		return path, nil
	}
	target, knownAlias := macOSRootAliases[components[0]]
	if !knownAlias {
		return path, nil
	}
	alias := filepath.Join(string(filepath.Separator), components[0])
	info, err := os.Lstat(alias)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	actual, err := os.Readlink(alias)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(actual) {
		actual = filepath.Join(string(filepath.Separator), actual)
	}
	if filepath.Clean(actual) != target {
		return "", fmt.Errorf("unexpected macOS root alias %s target", alias)
	}
	return filepath.Join(append([]string{target}, components[1:]...)...), nil
}

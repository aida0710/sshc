package sftp

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// localMutationTarget pins the resolved parent, while leaving the final
// component unresolved. Removing or renaming a link acts on the link itself.
type localMutationTarget struct {
	parent     *os.Root
	name       string
	publicPath string
	absolute   string
}

func isLocalTemporaryName(name string) bool {
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	return isInternalName(name) || strings.HasPrefix(name, ".sshc-download-")
}

func cleanLocalMutationPath(value string) (string, error) {
	if value == "" || hasLocalTraversalSegment(value) {
		return "", ErrInvalidPath
	}
	cleaned, err := cleanLocalPath(value)
	if err != nil {
		return "", err
	}
	native := filepath.FromSlash(cleaned)
	volume := filepath.VolumeName(native)
	if runtime.GOOS == "windows" && (strings.HasPrefix(native, `\\?\`) || strings.HasPrefix(native, `\\.\`)) {
		return "", ErrInvalidPath
	}
	relative := strings.TrimPrefix(native, volume+string(filepath.Separator))
	if relative == "" || native == volume {
		return "", ErrRootOperation
	}
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if !validLocalMutationName(component) {
			return "", ErrInvalidPath
		}
	}
	return cleaned, nil
}

func hasLocalTraversalSegment(value string) bool {
	// Home expansion must not silently erase traversal supplied by the user.
	for _, component := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if component == "." || component == ".." {
			return true
		}
	}
	return false
}

func openLocalMutationTarget(value string) (localMutationTarget, error) {
	cleaned, err := cleanLocalMutationPath(value)
	if err != nil {
		return localMutationTarget{}, err
	}
	filesystem, relative, err := openLocalRoot(cleaned)
	if err != nil {
		return localMutationTarget{}, err
	}
	defer filesystem.Close()
	if _, err := checkLocal(filesystem, path.Dir(relative), false); err != nil {
		return localMutationTarget{}, err
	}
	parent, err := filesystem.OpenRoot(path.Dir(relative))
	if err != nil {
		return localMutationTarget{}, err
	}
	target := localMutationTarget{parent: parent, name: path.Base(relative), publicPath: cleaned,
		absolute: filepath.ToSlash(filepath.Join(parent.Name(), filepath.FromSlash(path.Base(relative))))}
	if err := refuseLocalHomeAncestor(target.absolute); err != nil {
		parent.Close()
		return localMutationTarget{}, err
	}
	return target, nil
}

func refuseLocalHomeAncestor(candidate string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if localSameOrDescendant(candidate, filepath.ToSlash(filepath.Clean(home))) {
		return ErrRootOperation
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return err
	}
	if localSameOrDescendant(candidate, filepath.ToSlash(resolved)) {
		// A symlink naming home remains a link; its target is never removed.
		metadata, err := os.Lstat(filepath.FromSlash(candidate))
		if err == nil && metadata.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		return ErrRootOperation
	}
	return nil
}

// filepath.Join retains UNC roots, unlike the slash-only path.Join.
func joinLocalMutationPath(directory, name string) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(directory), name))
}

func localMutationEntry(publicPath string, metadata fs.FileInfo) Entry {
	entry := entryFrom("", metadata)
	entry.Path = publicPath
	return entry
}

func localSameOrDescendant(folder, candidate string) bool {
	if runtime.GOOS == "windows" {
		folder, candidate = strings.ToLower(folder), strings.ToLower(candidate)
	}
	return isSameOrDescendant(folder, candidate)
}

func localPathsOverlap(left, right string) bool {
	return localSameOrDescendant(left, right) || localSameOrDescendant(right, left)
}

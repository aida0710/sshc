package sftp

import (
	"context"
	"os"
	"path"
	"path/filepath"
)

// The tab identity cannot collide with an SSH alias, which rejects ':' and '/'.
const localComparisonAlias = "sshc://local"

func cleanComparisonPath(alias, value string) (string, error) {
	if alias == localComparisonAlias {
		return cleanLocalPath(value)
	}
	return cleanPublicPath(value, true)
}

// Hold the selected directory open so that a replaced child cannot redirect
// traversal outside it. Symlinks below this root are compared without following.
func readLocalComparisonTree(ctx context.Context, rootPath string) (map[string]Entry, error) {
	root, err := os.OpenRoot(filepath.FromSlash(rootPath))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries := make(map[string]Entry)
	pending := []string{"."}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := pending[0]
		pending = pending[1:]
		children, err := readLocalComparisonChildren(root, directory)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if isInternalName(child.Name()) {
				continue
			}
			relative := path.Join(directory, child.Name())
			info, err := root.Lstat(relative)
			if err != nil {
				return nil, err
			}
			absolute := filepath.Join(filepath.FromSlash(rootPath), filepath.FromSlash(relative))
			entry := entryFrom(filepath.ToSlash(filepath.Dir(absolute)), info)
			entry.Path = filepath.ToSlash(absolute)
			entries[relative] = entry
			if len(entries) > maxComparedEntries {
				return nil, ErrCompareLimit
			}
			if entry.Type == EntryDirectory {
				pending = append(pending, relative)
			}
		}
	}
	return entries, nil
}

func readLocalComparisonChildren(root *os.Root, directory string) ([]os.DirEntry, error) {
	opened, err := root.Open(directory)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	return opened.ReadDir(-1)
}

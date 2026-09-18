package sftp

import (
	"context"
	"path"
)

type deleteEntry struct {
	path      string
	directory bool
}

const (
	maxDeleteEntries = 200_000
	maxDeleteDepth   = 128
)

// collectDeleteTree walks without following symlinks and validates the entire
// tree before removing anything. The postorder list keeps directories last.
func collectDeleteTree(ctx context.Context, remote Remote, root string) ([]deleteEntry, error) {
	entries := make([]deleteEntry, 0, 16)
	var walk func(string, int) error
	walk = func(candidate string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > maxDeleteDepth || len(entries) >= maxDeleteEntries {
			return ErrCompareLimit
		}
		info, err := remote.Lstat(candidate)
		if err != nil {
			return err
		}
		if info.IsDir() {
			children, err := readChildren(ctx, remote, candidate)
			if err != nil {
				return err
			}
			for _, child := range children {
				if isInternalName(child.Name()) {
					return ErrConflict
				}
				if err := walk(path.Join(candidate, child.Name()), depth+1); err != nil {
					return err
				}
			}
		}
		entries = append(entries, deleteEntry{path: candidate, directory: info.IsDir()})
		if len(entries) > maxDeleteEntries {
			return ErrCompareLimit
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s Service) PlanDelete(ctx context.Context, alias, remotePath string) (int64, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return 0, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return 0, err
	}
	defer remote.Close()
	entries, err := collectDeleteTree(ctx, remote, cleaned)
	return int64(len(entries)), err
}

func (s Service) DeleteWithProgress(ctx context.Context, alias, remotePath string, expectedItems int64, progress func(int64) error) error {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return err
	}
	defer remote.Close()
	entries, err := collectDeleteTree(ctx, remote, cleaned)
	if err != nil {
		return err
	}
	if expectedItems >= 0 && int64(len(entries)) != expectedItems {
		return ErrConflict
	}
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.directory {
			err = remote.RemoveDirectory(entry.path)
		} else {
			err = remote.Remove(entry.path)
		}
		if err != nil {
			return err
		}
		// The last entry is the requested root. Completion records its final
		// progress, so a progress-write failure cannot leave a fully removed
		// tree looking like a retryable failed delete.
		if progress != nil && index < len(entries)-1 {
			if err := progress(int64(index + 1)); err != nil {
				return err
			}
		}
	}
	return nil
}

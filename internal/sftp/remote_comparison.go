package sftp

import (
	"context"
	"io/fs"
)

// The caller owns the remote transport, including on traversal failure.
func readRemoteComparisonTree(ctx context.Context, remote Remote, root string) (*comparisonTree, error) {
	tree := &comparisonTree{remote: remote, rootPath: root, entries: make(map[string]Entry), metadata: make(map[string]fs.FileInfo)}
	result := tree.entries
	pending := []string{root}
	visited := 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := pending[0]
		pending = pending[1:]
		observed, err := remote.Lstat(directory)
		if err != nil {
			return nil, err
		}
		tree.metadata[comparisonRelativePath(root, directory)] = observed
		infos, err := readStableRemoteDirectory(ctx, remote, directory)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			visited++
			if visited > maxComparedEntries {
				return nil, ErrCompareLimit
			}
			if isInternalName(info.Name()) {
				continue
			}
			entry := entryFrom(directory, info)
			relative := entry.Path[len(root):]
			if len(relative) > 0 && relative[0] == '/' {
				relative = relative[1:]
			}
			result[relative] = entry
			tree.metadata[relative] = info
			if len(result) > maxComparedEntries {
				return nil, ErrCompareLimit
			}
			if entry.Type == EntryDirectory {
				pending = append(pending, entry.Path)
			}
		}
	}
	return tree, nil
}

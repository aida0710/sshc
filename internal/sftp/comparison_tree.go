package sftp

import (
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Comparison trees retain their connections and local roots until hashes finish.
type comparisonTree struct {
	entries   map[string]Entry
	metadata  map[string]fs.FileInfo
	remote    Remote
	localRoot *os.Root
	rootPath  string
	checked   []string
}

func (tree *comparisonTree) close() {
	if tree.remote != nil {
		tree.remote.Close()
	}
	if tree.localRoot != nil {
		tree.localRoot.Close()
	}
}

func (tree *comparisonTree) hash(ctx context.Context, relative string, budget *contentReadBudget) (string, error) {
	var file stableContentFile
	var err error
	if tree.localRoot != nil {
		file, err = openStableLocalFile(localContentRead{root: tree.localRoot, relative: relative, metadata: tree.metadata})
	} else {
		if err := tree.verifyRemoteEntry(relative); err != nil {
			return "", err
		}
		file, err = openStableRemoteFile(tree.remote, tree.entries[relative])
	}
	if err != nil {
		return "", err
	}
	defer file.reader.Close()
	digest := sha256.New()
	if err := budget.stream(ctx, contentStream{file: file, destination: digest}); err != nil {
		return "", err
	}
	tree.checked = append(tree.checked, relative)
	return contentRevisionOf(digest), nil
}

// A side may change while the other side is being read. Recheck the paths
// whose hashes were used before reporting the comparison as complete.
func (tree *comparisonTree) verify(ctx context.Context) error {
	if tree.localRoot != nil {
		current, err := os.Lstat(filepath.FromSlash(tree.rootPath))
		if err != nil || !os.SameFile(current, tree.metadata["."]) {
			return ErrConflict
		}
	}
	for _, relative := range tree.checked {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tree.localRoot != nil {
			if err := tree.verifyLocalEntry(relative); err != nil {
				return err
			}
			continue
		}
		if err := tree.verifyRemoteEntry(relative); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (tree *comparisonTree) verifyLocalEntry(relative string) error {
	current, err := inspectLocalReadPath(tree.localRoot, relative)
	if err != nil {
		return ErrConflict
	}
	for candidate, info := range current {
		expected, exists := tree.metadata[candidate]
		if exists && (!os.SameFile(info, expected) || metadataRevision(info) != metadataRevision(expected)) {
			return ErrConflict
		}
	}
	return nil
}

func (tree *comparisonTree) verifyRemoteEntry(relative string) error {
	metadata, err := inspectRemoteReadPath(tree.remote, tree.entries[relative].Path)
	if err != nil {
		return ErrConflict
	}
	for candidate, current := range metadata {
		if expected, exists := tree.metadata[comparisonRelativePath(tree.rootPath, candidate)]; exists && metadataRevision(current) != metadataRevision(expected) {
			return ErrConflict
		}
	}
	return nil
}

func comparisonRelativePath(root, candidate string) string {
	if candidate == root {
		return "."
	}
	// Ancestors outside the selected tree retain their absolute names, so they
	// cannot collide with a relative entry or the selected root's snapshot.
	return strings.TrimPrefix(candidate, strings.TrimSuffix(root, "/")+"/")
}

package sftp

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// The tab identity cannot collide with an SSH alias, which rejects ':' and '/'.
const localComparisonAlias = "sshc://local"

// Small batches bound listing memory and let cancellation interrupt large directories.
const localComparisonBatchSize = 128

type localComparisonDirectoryReader interface {
	ReadDir(int) ([]os.DirEntry, error)
}

type localComparisonWalker struct {
	root               *os.Root
	rootPath           string
	entries            map[string]Entry
	pendingDirectories []string
	scannedEntryCount  int
	metadata           map[string]fs.FileInfo
}

func cleanComparisonPath(alias, value string) (string, error) {
	if alias == localComparisonAlias {
		return cleanLocalPath(value)
	}
	return cleanPublicPath(value, true)
}

// Hold the selected directory open so that a replaced child cannot redirect
// traversal outside it. Symlinks below this root are compared without following.
func openLocalComparisonTree(ctx context.Context, rootPath string, mode ComparisonMode) (tree *comparisonTree, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	var expectedRoot fs.FileInfo
	if mode == ComparisonContent {
		expectedRoot, err = os.Lstat(filepath.FromSlash(rootPath))
		if err != nil {
			return nil, err
		}
		if !expectedRoot.IsDir() {
			return nil, ErrNotDirectory
		}
	}
	root, err := os.OpenRoot(filepath.FromSlash(rootPath))
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			root.Close()
		}
	}()
	walker := localComparisonWalker{
		root: root, rootPath: rootPath,
		entries: make(map[string]Entry), pendingDirectories: []string{"."},
	}
	if mode == ComparisonContent {
		walker.metadata = make(map[string]fs.FileInfo)
		walker.metadata["."], err = root.Lstat(".")
		if err != nil {
			return nil, err
		}
		if !os.SameFile(expectedRoot, walker.metadata["."]) {
			return nil, ErrConflict
		}
	}
	entries, err := walker.walk(ctx)
	if err != nil {
		return nil, err
	}
	succeeded = true
	return &comparisonTree{entries: entries, metadata: walker.metadata, localRoot: root, rootPath: rootPath}, nil
}

func readLocalComparisonTree(ctx context.Context, rootPath string) (map[string]Entry, error) {
	tree, err := openLocalComparisonTree(ctx, rootPath, ComparisonMetadata)
	if err != nil {
		return nil, err
	}
	defer tree.close()
	return tree.entries, nil
}

func (walker *localComparisonWalker) walk(ctx context.Context) (map[string]Entry, error) {
	for len(walker.pendingDirectories) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := walker.pendingDirectories[0]
		walker.pendingDirectories = walker.pendingDirectories[1:]
		if err := walker.walkDirectory(ctx, directory); err != nil {
			return nil, err
		}
	}
	return walker.entries, nil
}

func (walker *localComparisonWalker) walkDirectory(ctx context.Context, directory string) error {
	metadata, err := inspectLocalReadPath(walker.root, directory)
	if err != nil {
		return err
	}
	opened, err := walker.root.Open(directory)
	if err != nil {
		return err
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil || !os.SameFile(info, metadata[directory]) {
		return ErrConflict
	}
	if err := walker.walkDirectoryEntries(ctx, directory, opened); err != nil {
		return err
	}
	return verifyLocalReadPath(walker.root, metadata)
}

func (walker *localComparisonWalker) walkDirectoryEntries(ctx context.Context, directory string, reader localComparisonDirectoryReader) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Read at most one entry beyond the tree budget to distinguish a full
		// tree from an oversized one, including names hidden from comparisons.
		batchSize := min(localComparisonBatchSize, maxComparedEntries-walker.scannedEntryCount+1)
		children, readErr := reader.ReadDir(batchSize)
		if err := ctx.Err(); err != nil {
			return err
		}
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		walker.scannedEntryCount += len(children)
		if walker.scannedEntryCount > maxComparedEntries {
			return ErrCompareLimit
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := walker.recordChild(directory, child); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
	}
}

func (walker *localComparisonWalker) recordChild(directory string, child os.DirEntry) error {
	if isInternalName(child.Name()) {
		return nil
	}
	relative := path.Join(directory, child.Name())
	info, err := walker.root.Lstat(relative)
	if err != nil {
		return err
	}
	absolute := filepath.Join(filepath.FromSlash(walker.rootPath), filepath.FromSlash(relative))
	entry := entryFrom(filepath.ToSlash(filepath.Dir(absolute)), info)
	entry.Path = filepath.ToSlash(absolute)
	walker.entries[relative] = entry
	if walker.metadata != nil {
		walker.metadata[relative] = info
	}
	if entry.Type == EntryDirectory {
		walker.pendingDirectories = append(walker.pendingDirectories, relative)
	}
	return nil
}

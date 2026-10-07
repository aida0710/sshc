package sftp

import (
	"context"
	"path"
	"sort"
)

// maxComparedEntries bounds how many entries a directory comparison reads from
// both sides together.
const maxComparedEntries = 20_000

// CompareDirectories compares metadata without downloading file contents.
// Symlinks are listed but never followed.
func (s Service) CompareDirectories(ctx context.Context, leftAlias, leftPath, rightAlias, rightPath string) (DirectoryComparison, error) {
	leftRoot, err := cleanComparisonPath(leftAlias, leftPath)
	if err != nil {
		return DirectoryComparison{}, err
	}
	rightRoot, err := cleanComparisonPath(rightAlias, rightPath)
	if err != nil {
		return DirectoryComparison{}, err
	}
	left, err := s.readComparisonTree(ctx, leftAlias, leftRoot)
	if err != nil {
		return DirectoryComparison{}, err
	}
	right, err := s.readComparisonTree(ctx, rightAlias, rightRoot)
	if err != nil {
		return DirectoryComparison{}, err
	}
	keys := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for candidate := range left {
		seen[candidate] = struct{}{}
		keys = append(keys, candidate)
	}
	for candidate := range right {
		if _, ok := seen[candidate]; ok {
			continue
		}
		keys = append(keys, candidate)
	}
	if len(keys) > maxComparedEntries {
		return DirectoryComparison{}, ErrCompareLimit
	}
	sort.Strings(keys)
	result := DirectoryComparison{LeftPath: leftRoot, RightPath: rightRoot, Entries: make([]DirectoryDifference, 0, len(keys))}
	entryIndex := make(map[string]int, len(keys))
	for _, relative := range keys {
		leftEntry, leftOK := left[relative]
		rightEntry, rightOK := right[relative]
		difference := DirectoryDifference{RelativePath: relative}
		if leftOK {
			copy := leftEntry
			difference.Left = &copy
		}
		if rightOK {
			copy := rightEntry
			difference.Right = &copy
		}
		switch {
		case !rightOK:
			difference.Status = DirectoryLeftOnly
		case !leftOK:
			difference.Status = DirectoryRightOnly
		case leftEntry.Type != rightEntry.Type:
			difference.Status = DirectoryTypeMismatch
		case leftEntry.Type == EntryDirectory:
			difference.Status = DirectorySame
		case leftEntry.Size == rightEntry.Size && leftEntry.Mode.Perm() == rightEntry.Mode.Perm() && leftEntry.ModifiedAt.Equal(rightEntry.ModifiedAt):
			difference.Status = DirectorySame
		default:
			difference.Status = DirectoryDifferent
		}
		entryIndex[relative] = len(result.Entries)
		result.Entries = append(result.Entries, difference)
	}
	// A directory is different when any descendant differs. This lets the UI
	// summarize a large tree without pretending equal directory mtimes imply
	// equal contents.
	for index := len(result.Entries) - 1; index >= 0; index-- {
		item := result.Entries[index]
		if item.Status == DirectorySame || item.RelativePath == "" {
			continue
		}
		parent := path.Dir(item.RelativePath)
		for parent != "." && parent != "/" {
			if parentIndex, ok := entryIndex[parent]; ok {
				candidate := &result.Entries[parentIndex]
				if candidate.Status == DirectorySame {
					candidate.Status = DirectoryDifferent
				}
			}
			parent = path.Dir(parent)
		}
	}
	return result, nil
}

func (s Service) readComparisonTree(ctx context.Context, alias, root string) (map[string]Entry, error) {
	if alias == localComparisonAlias {
		return readLocalComparisonTree(ctx, root)
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer remote.Close()
	result := make(map[string]Entry)
	pending := []string{root}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := pending[0]
		pending = pending[1:]
		infos, err := readChildren(ctx, remote, directory)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			if isInternalName(info.Name()) {
				continue
			}
			entry := entryFrom(directory, info)
			relative := entry.Path[len(root):]
			if len(relative) > 0 && relative[0] == '/' {
				relative = relative[1:]
			}
			result[relative] = entry
			if len(result) > maxComparedEntries {
				return nil, ErrCompareLimit
			}
			if entry.Type == EntryDirectory {
				pending = append(pending, entry.Path)
			}
		}
	}
	return result, nil
}

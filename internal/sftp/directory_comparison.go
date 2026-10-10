package sftp

import (
	"context"
	"sort"
)

// maxComparedEntries bounds each directory tree and the combined comparison list.
const maxComparedEntries = 20_000

// CompareDirectories compares metadata by default or streams bounded content hashes.
// Symlinks are listed but never followed.
func (s Service) CompareDirectories(ctx context.Context, options CompareOptions) (DirectoryComparison, error) {
	if options.Mode == "" {
		options.Mode = ComparisonMetadata
	}
	if options.Mode != ComparisonMetadata && options.Mode != ComparisonContent {
		return DirectoryComparison{}, ErrInvalidQuery
	}
	leftAlias, leftPath := options.Left.Alias, options.Left.Path
	rightAlias, rightPath := options.Right.Alias, options.Right.Path
	leftRoot, err := cleanComparisonPath(leftAlias, leftPath)
	if err != nil {
		return DirectoryComparison{}, err
	}
	rightRoot, err := cleanComparisonPath(rightAlias, rightPath)
	if err != nil {
		return DirectoryComparison{}, err
	}
	leftTree, err := s.readComparisonTree(ctx, ComparisonLocation{Alias: leftAlias, Path: leftRoot}, options.Mode)
	if err != nil {
		return DirectoryComparison{}, err
	}
	defer leftTree.close()
	var rightTree *comparisonTree
	sharesRemote := leftAlias == rightAlias && leftAlias != localComparisonAlias
	if sharesRemote {
		// Reuse one SFTP transport, including hosts allowing only one OTP connection.
		rightTree, err = readRemoteComparisonTree(ctx, leftTree.remote, rightRoot)
	} else {
		rightTree, err = s.readComparisonTree(ctx, ComparisonLocation{Alias: rightAlias, Path: rightRoot}, options.Mode)
	}
	if err != nil {
		return DirectoryComparison{}, err
	}
	if !sharesRemote {
		defer rightTree.close()
	}
	left, right := leftTree.entries, rightTree.entries
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
	result := DirectoryComparison{LeftPath: leftRoot, RightPath: rightRoot, Mode: options.Mode, Entries: make([]DirectoryDifference, 0, len(keys))}
	content := contentComparison{left: leftTree, right: rightTree, budget: contentReadBudget{maxBytes: maxContentComparisonBytes}}
	entryIndex := make(map[string]int, len(keys))
	for _, relative := range keys {
		if err := ctx.Err(); err != nil {
			return DirectoryComparison{}, err
		}
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
		if options.Mode == ComparisonContent {
			if err := content.compare(ctx, &difference); err != nil {
				return DirectoryComparison{}, err
			}
			if difference.Status == DirectoryUnverified {
				result.Truncated = true
			}
		}
		entryIndex[relative] = len(result.Entries)
		result.Entries = append(result.Entries, difference)
	}
	propagateComparisonDifferences(result.Entries, entryIndex)
	if options.Mode == ComparisonContent {
		if err := leftTree.verify(ctx); err != nil {
			return DirectoryComparison{}, err
		}
		if err := rightTree.verify(ctx); err != nil {
			return DirectoryComparison{}, err
		}
		result.BytesRead = content.budget.bytesRead
	}
	return result, nil
}

func (s Service) readComparisonTree(ctx context.Context, location ComparisonLocation, mode ComparisonMode) (*comparisonTree, error) {
	alias, root := location.Alias, location.Path
	if alias == localComparisonAlias {
		return openLocalComparisonTree(ctx, root, mode)
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return nil, err
	}
	tree, err := readRemoteComparisonTree(ctx, remote, root)
	if err != nil {
		remote.Close()
	}
	return tree, err
}

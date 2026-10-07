package sftp

import "context"

type contentComparison struct {
	left   *comparisonTree
	right  *comparisonTree
	budget contentReadBudget
}

func (comparison *contentComparison) compare(ctx context.Context, difference *DirectoryDifference) error {
	if difference.Left == nil || difference.Right == nil || difference.Left.Type != difference.Right.Type || difference.Left.Type == EntryDirectory {
		return nil
	}
	if difference.Left.Type != EntryFile {
		difference.Status = DirectoryUnverified
		difference.Omission = omissionUnsupported
		return nil
	}
	if difference.Left.Size != difference.Right.Size {
		difference.Status = DirectoryDifferent
		return nil
	}
	size := difference.Left.Size
	if size < 0 || size > (comparison.budget.maxBytes-comparison.budget.bytesRead)/2 {
		difference.Status = DirectoryUnverified
		difference.Omission = omissionByteLimit
		return nil
	}
	leftHash, err := comparison.left.hash(ctx, difference.RelativePath, &comparison.budget)
	if err != nil {
		return err
	}
	rightHash, err := comparison.right.hash(ctx, difference.RelativePath, &comparison.budget)
	if err != nil {
		return err
	}
	difference.Status = DirectorySame
	if leftHash != rightHash {
		difference.Status = DirectoryDifferent
	}
	return nil
}

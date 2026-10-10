package sftp

import "path"

// A parent's summary must retain unknown descendants even when another child
// differs. Otherwise selecting that parent could implicitly copy unchecked files.
func propagateComparisonDifferences(entries []DirectoryDifference, entryIndex map[string]int) {
	for index := len(entries) - 1; index >= 0; index-- {
		markComparisonParents(entries[index], entries, entryIndex)
	}
}

func markComparisonParents(difference DirectoryDifference, entries []DirectoryDifference, entryIndex map[string]int) {
	if difference.Status == DirectorySame || difference.RelativePath == "" {
		return
	}
	for parent := path.Dir(difference.RelativePath); parent != "." && parent != "/"; parent = path.Dir(parent) {
		parentIndex, exists := entryIndex[parent]
		if !exists {
			continue
		}
		candidate := &entries[parentIndex]
		if difference.Status == DirectoryUnverified {
			candidate.Status, candidate.Omission = DirectoryUnverified, difference.Omission
			continue
		}
		if candidate.Status == DirectorySame {
			candidate.Status = DirectoryDifferent
		}
	}
}

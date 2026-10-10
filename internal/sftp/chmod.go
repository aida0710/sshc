package sftp

import (
	"io/fs"
)

// Match the API selection limit and keep one confirmation manageable.
const MaxChmodSelection = 200

// Share the existing recursive chmod traversal budget across all selected trees.
const maxChmodPlanEntries = maxSearchVisited

// Distinguish complete permission plans from single-entry metadata revisions.
const chmodRevisionPrefix = "chmod-sha256:"

type ChmodEntry struct {
	Path             string
	ExpectedRevision string
}

type ChmodOptions struct {
	FileMode      fs.FileMode
	DirectoryMode fs.FileMode
	Recursive     bool
}

type ChmodRequest struct {
	Alias   string
	Entries []ChmodEntry
	Options ChmodOptions
}

type ChmodResult struct {
	Applied int
	Items   int
	Started bool
}

// ChmodRemote changes only the named entry, even if it is replaced by a link.
// Ordinary Chmod remains available for engine-created transfer temporary files.
type ChmodRemote interface {
	CheckChmodNoFollow() error
	ChmodNoFollow(path string, mode fs.FileMode) error
}

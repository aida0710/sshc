package sftp

import (
	"fmt"
	"path"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// publishedNames remembers, for one run of a transfer, the names the run has
// written into each target directory. A case-insensitive file system (APFS and
// NTFS by default, and SFTP servers on them) resolves two source names that
// differ only by case or Unicode normalization, such as xt_CONNMARK.h and
// xt_connmark.h, to one target entry. Without this record the second name
// looks like an entry that was already at the target, and an approved
// overwrite silently replaces what the run itself wrote a moment ago.
//
// get, put, copy and move all decide about an existing target entry through
// existingEntryError, so the rule is the same whichever side is local.
type publishedNames struct {
	// directories maps a target directory to the name keys written into it,
	// each with the exact name that was written.
	directories map[string]map[string]string
	// fold is not safe for concurrent use; a run walks its tree on one
	// goroutine.
	fold cases.Caser
}

func newPublishedNames() *publishedNames {
	return &publishedNames{directories: make(map[string]map[string]string), fold: cases.Fold()}
}

// nameKey folds a name the way a case-insensitive file system compares names:
// Unicode case folding on the NFC form, since APFS also treats NFC and NFD
// spellings as one name. It may equate more names than a particular file
// system does; such a collision stops the job, it never loses data.
func (p *publishedNames) nameKey(name string) string {
	return norm.NFC.String(p.fold.String(norm.NFC.String(name)))
}

// record notes that the run wrote target, as a file it published or as a
// folder it created or writes into.
func (p *publishedNames) record(target string) {
	directory := path.Dir(target)
	names := p.directories[directory]
	if names == nil {
		names = make(map[string]string)
		p.directories[directory] = names
	}
	names[p.nameKey(path.Base(target))] = path.Base(target)
}

// existingEntryError decides whether the run may write over an entry that
// exists at target. An entry that resolves to a name this run already wrote
// is a name collision, which no overwrite approval covers: the approval is for
// entries that were at the target before the run. Any other existing entry
// needs that approval.
func (p *publishedNames) existingEntryError(target string, overwrite bool) error {
	if written, ok := p.directories[path.Dir(target)][p.nameKey(path.Base(target))]; ok {
		return fmt.Errorf("%w: %q and %q in %s", ErrNameCollision, written, path.Base(target), path.Dir(target))
	}
	if !overwrite {
		return ErrAlreadyExists
	}
	return nil
}

// differentKindError is the failure of an approved overwrite whose existing
// target is another kind of entry than the one replacing it, such as a file
// where a folder goes. Approving the overwrite again would not help, so the
// run fails as a conflict instead of asking for it with ErrAlreadyExists.
func differentKindError(target string) error {
	return fmt.Errorf("%w: %s is a different kind of entry than the one replacing it", ErrConflict, target)
}

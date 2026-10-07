package sftp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// A short basename keeps the temporary within server filename limits even for long link names.
const symlinkTemporaryBaseName = "link"

// SymlinkChange changes the link itself, including a dangling link.
type SymlinkChange struct {
	Path             string
	Target           string
	ExpectedRevision string
}

func (s Service) CreateSymlink(ctx context.Context, alias string, change SymlinkChange) (Entry, error) {
	cleaned, err := cleanMetadataPath(change.Path)
	if err != nil {
		return Entry{}, err
	}
	if err := validateLinkTarget(change.Target); err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	links, supported := optionalRemote(remote).(SymlinkRemote)
	if !supported {
		return Entry{}, ErrUnsupportedOperation
	}
	if _, err := remote.Lstat(cleaned); err == nil {
		return Entry{}, ErrAlreadyExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	// SSH_FXP_SYMLINK creates the link exclusively; never replace a raced-in entry.
	if err := links.Symlink(change.Target, cleaned); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrAlreadyExists
		}
		return Entry{}, err
	}
	return statLink(remote, cleaned)
}

func (s Service) ChangeSymlink(ctx context.Context, alias string, change SymlinkChange) (entry Entry, changeErr error) {
	if change.ExpectedRevision == "" {
		return Entry{}, ErrRevisionRequired
	}
	cleaned, err := cleanMetadataPath(change.Path)
	if err != nil {
		return Entry{}, err
	}
	if err := validateLinkTarget(change.Target); err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	links, supported := optionalRemote(remote).(SymlinkRemote)
	if !supported {
		return Entry{}, ErrUnsupportedOperation
	}
	atomicLinks, supported := optionalRemote(remote).(AtomicSymlinkRemote)
	if !supported {
		return Entry{}, ErrUnsupportedOperation
	}
	if err := verifyLink(remote, cleaned, change.ExpectedRevision); err != nil {
		return Entry{}, err
	}
	temporary, err := s.temporaryPath(path.Join(path.Dir(cleaned), symlinkTemporaryBaseName))
	if err != nil {
		return Entry{}, err
	}
	if temporary == cleaned || path.Dir(temporary) != path.Dir(cleaned) || !isInternalName(path.Base(temporary)) {
		return Entry{}, ErrInvalidPath
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if err := links.Symlink(change.Target, temporary); err != nil {
		return Entry{}, err
	}
	published := false
	defer func() {
		if !published {
			changeErr = errors.Join(changeErr, (unpublishedFile{service: s, alias: alias, remote: remote, path: temporary}).remove(ctx))
		}
	}()
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if err := verifyLink(remote, cleaned, change.ExpectedRevision); err != nil {
		return Entry{}, err
	}
	if err := atomicLinks.ReplaceSymlink(temporary, cleaned); err != nil {
		return Entry{}, err
	}
	published = true
	return statLink(remote, cleaned)
}

func verifyLink(remote Remote, candidate, expectedRevision string) error {
	entry, err := statLink(remote, candidate)
	if err != nil {
		return err
	}
	if entry.Revision != expectedRevision {
		return ErrConflict
	}
	return nil
}

func statLink(remote Remote, candidate string) (Entry, error) {
	info, err := remote.Lstat(candidate)
	if err != nil {
		return Entry{}, err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return Entry{}, ErrNotSymlink
	}
	target, err := remote.ReadLink(candidate)
	if err != nil {
		return Entry{}, err
	}
	entry := entryFrom(path.Dir(candidate), namedInfo{FileInfo: info, name: path.Base(candidate)})
	entry.LinkTarget = target
	entry.Revision = symlinkRevision(entry.Revision, target)
	return entry, nil
}

// The target must participate even when a same-length retarget leaves size/mtime unchanged.
func symlinkRevision(metadata, target string) string {
	return fmt.Sprintf("%s:link:%s", metadata, contentRevision([]byte(target)))
}

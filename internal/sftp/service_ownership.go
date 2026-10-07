package sftp

import (
	"context"
	"path"
)

// OwnershipChange sets both SFTP v3 owner attributes without interpreting account names.
type OwnershipChange struct {
	Path             string
	UID              uint32
	GID              uint32
	ExpectedRevision string
}

func (s Service) ChangeOwnership(ctx context.Context, alias string, change OwnershipChange) (Entry, error) {
	if change.ExpectedRevision == "" {
		return Entry{}, ErrRevisionRequired
	}
	cleaned, err := cleanMetadataPath(change.Path)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	ownerRemote, supported := optionalRemote(remote).(OwnershipRemote)
	if !supported {
		return Entry{}, ErrUnsupportedOperation
	}
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	if !metadataTypeKnown(info) {
		return Entry{}, ErrMetadataUnavailable
	}
	// The no-follow mutation protects against a later link replacement; links
	// already present are refused because this action is for files/directories.
	if !info.Mode().IsRegular() && !info.IsDir() {
		return Entry{}, ErrNotRegularFile
	}
	if _, available := ownershipFrom(info); !available {
		return Entry{}, ErrOwnershipUnavailable
	}
	if metadataRevision(info) != change.ExpectedRevision {
		return Entry{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if err := ownerRemote.Chown(cleaned, change.UID, change.GID); err != nil {
		return Entry{}, err
	}
	updated, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	if !metadataTypeKnown(updated) || (!updated.Mode().IsRegular() && !updated.IsDir()) {
		return Entry{}, ErrConflict
	}
	owner, available := ownershipFrom(updated)
	if !available {
		return Entry{}, ErrOwnershipUnavailable
	}
	if owner.UID != change.UID || owner.GID != change.GID {
		return Entry{}, ErrUnsupportedOperation
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: updated, name: path.Base(cleaned)}), nil
}

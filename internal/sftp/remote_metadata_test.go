package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path"
	"strings"
	"testing"

	pkgsftp "github.com/pkg/sftp"
	"sshc/internal/sftp"
)

type ownedMetadata struct {
	fs.FileInfo
	owner     sftp.Ownership
	available bool
}

func (info ownedMetadata) Ownership() (sftp.Ownership, bool) { return info.owner, info.available }

type metadataRemote struct {
	*fakeRemote
	owners            map[string]sftp.Ownership
	symlinkErr        error
	chownErr          error
	afterSymlink      func()
	stats             *pkgsftp.StatVFS
	spaceErr          error
	chownPaths        []string
	maxFilenameBytes  int
	preserveOwnership bool
	beforeChown       func()
}

func metadataFixture() *metadataRemote {
	return &metadataRemote{fakeRemote: remoteWith(linkedTree()), owners: map[string]sftp.Ownership{
		"/srv/notes.txt": {UID: 1000, GID: 100}, "/srv/data": {UID: 1000, GID: 100},
	}}
}
func (remote *metadataRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, err := remote.fakeRemote.Lstat(candidate)
	if err != nil {
		return nil, err
	}
	owner, available := remote.owners[candidate]
	return ownedMetadata{FileInfo: info, owner: owner, available: available}, nil
}
func (remote *metadataRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	children, err := remote.fakeRemote.ReadDir(ctx, directory)
	for index, info := range children {
		owner, available := remote.owners[path.Join(directory, info.Name())]
		children[index] = ownedMetadata{FileInfo: info, owner: owner, available: available}
	}
	return children, err
}
func (remote *metadataRemote) Symlink(target, linkPath string) error {
	if remote.maxFilenameBytes > 0 && len(path.Base(linkPath)) > remote.maxFilenameBytes {
		return fs.ErrInvalid
	}
	if remote.symlinkErr != nil {
		return remote.symlinkErr
	}
	if _, exists := remote.nodes[linkPath]; exists {
		return fs.ErrExist
	}
	remote.nodes[linkPath] = symlink(path.Base(linkPath), target)
	if remote.afterSymlink != nil {
		remote.afterSymlink()
	}
	return nil
}
func (remote *metadataRemote) ReplaceSymlink(temporary, linkPath string) error {
	return remote.Replace(temporary, linkPath)
}
func (remote *metadataRemote) Chown(candidate string, uid, gid uint32) error {
	if remote.beforeChown != nil {
		remote.beforeChown()
	}
	if remote.chownErr != nil {
		return remote.chownErr
	}
	remote.chownPaths = append(remote.chownPaths, candidate)
	if !remote.preserveOwnership {
		remote.owners[candidate] = sftp.Ownership{UID: uid, GID: gid}
	}
	return nil
}

func TestOwnershipRejectsAnEntryReplacedByALinkAndLeavesItsTargetUnchanged(t *testing.T) {
	remote := metadataFixture()
	service := metadataService(remote)
	before, err := service.Stat(t.Context(), "edge", "/srv/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	targetOwner := remote.owners["/srv/data"]
	remote.beforeChown = func() {
		remote.nodes[before.Path] = symlink("notes.txt", "/srv/data")
	}
	_, err = service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: before.Path, UID: 7, GID: 8, ExpectedRevision: before.Revision})
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("replaced entry = %v", err)
	}
	if remote.owners["/srv/data"] != targetOwner {
		t.Fatal("link target ownership changed")
	}
}
func (remote *metadataRemote) StatVFS(string) (*pkgsftp.StatVFS, error) {
	return remote.stats, remote.spaceErr
}

func metadataService(remote sftp.Remote) sftp.Service {
	return sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
}

func TestCreatingLinksAcceptsRelativeAbsoluteAndMissingTargetsWithoutOverwrite(t *testing.T) {
	for _, target := range []string{"data", "/srv/notes.txt", "../missing"} {
		t.Run(target, func(t *testing.T) {
			remote := metadataFixture()
			service := metadataService(remote)
			entry, err := service.CreateSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: "/srv/new-link", Target: target})
			if err != nil || entry.LinkTarget != target || entry.Type != sftp.EntrySymlink {
				t.Fatalf("created link = %+v, %v", entry, err)
			}
			if _, err := service.CreateSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: "/srv/new-link", Target: "other"}); !errors.Is(err, sftp.ErrAlreadyExists) {
				t.Fatalf("overwrite = %v", err)
			}
			if string(remote.nodes["/srv/new-link"].content) != target || string(remote.nodes["/srv/notes.txt"].content) != "hello" {
				t.Fatal("existing entry or target changed")
			}
		})
	}
}

func TestChangingALinkReplacesOnlyTheLinkAndCleansTemporaryOnFailure(t *testing.T) {
	for _, replacementErr := range []error{nil, fs.ErrPermission, sftp.ErrUnsupportedOperation} {
		t.Run(stringError(replacementErr), func(t *testing.T) {
			remote := metadataFixture()
			service := metadataService(remote)
			before, err := service.Stat(t.Context(), "edge", "/srv/notes-link")
			if err != nil {
				t.Fatal(err)
			}
			remote.replaceErr = replacementErr
			entry, err := service.ChangeSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: before.Path, Target: "absent", ExpectedRevision: before.Revision})
			if !errors.Is(err, replacementErr) {
				t.Fatalf("change = %+v, %v", entry, err)
			}
			expectedTarget := "absent"
			if replacementErr != nil {
				expectedTarget = "/srv/notes.txt"
			}
			if string(remote.nodes[before.Path].content) != expectedTarget || string(remote.nodes["/srv/notes.txt"].content) != "hello" {
				t.Fatal("link or target has wrong content")
			}
			for candidate := range remote.nodes {
				if path.Ext(candidate) == ".tmp" {
					t.Fatalf("temporary link remains: %s", candidate)
				}
			}
		})
	}
}

func stringError(err error) string {
	if err == nil {
		return "success"
	}
	return err.Error()
}

func TestChangingALongLinkNameStaysWithinServerFilenameLimits(t *testing.T) {
	remote := metadataFixture()
	remote.maxFilenameBytes = 255
	linkName := strings.Repeat("a", remote.maxFilenameBytes)
	linkPath := path.Join("/srv", linkName)
	remote.nodes[linkPath] = symlink(linkName, "notes.txt")
	service := metadataService(remote)
	before, err := service.Stat(t.Context(), "edge", linkPath)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := service.ChangeSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: linkPath, Target: "data", ExpectedRevision: before.Revision})
	if err != nil || changed.LinkTarget != "data" {
		t.Fatalf("long link retarget = %+v, %v", changed, err)
	}
}

func TestRetargetConflictIncludesSameLengthTargetAndRechecksBeforeReplace(t *testing.T) {
	remote := metadataFixture()
	remote.nodes["/srv/broken"] = symlink("broken", "missing")
	service := metadataService(remote)
	before, err := service.Stat(t.Context(), "edge", "/srv/broken")
	if err != nil {
		t.Fatal(err)
	}
	remote.afterSymlink = func() { remote.nodes["/srv/broken"] = symlink("broken", "changed") }
	_, err = service.ChangeSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: before.Path, Target: "new", ExpectedRevision: before.Revision})
	if !errors.Is(err, sftp.ErrConflict) || len(remote.replacements) != 0 {
		t.Fatalf("raced change = %v, replacements = %v", err, remote.replacements)
	}
	if string(remote.nodes[before.Path].content) != "changed" {
		t.Fatal("concurrent retarget was lost")
	}
	for candidate := range remote.nodes {
		if path.Ext(candidate) == ".tmp" {
			t.Fatalf("temporary remains: %s", candidate)
		}
	}
}

func TestOwnershipIDsAppearInListAndRevisionAndSymlinkTargetsAreRefused(t *testing.T) {
	remote := metadataFixture()
	service := metadataService(remote)
	listing, err := service.ListDirectory(t.Context(), "edge", "/srv")
	if err != nil {
		t.Fatal(err)
	}
	var before sftp.Entry
	for _, entry := range listing.Entries {
		if entry.Name == "notes.txt" {
			before = entry
		}
	}
	if before.Ownership == nil || before.Ownership.UID != 1000 {
		t.Fatalf("listed owner = %+v", before.Ownership)
	}
	changed, err := service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: before.Path, UID: 0, GID: math.MaxUint32, ExpectedRevision: before.Revision})
	if err != nil || changed.Ownership == nil || changed.Ownership.UID != 0 || changed.Ownership.GID != math.MaxUint32 || changed.Revision == before.Revision {
		t.Fatalf("changed owner = %+v, %v", changed, err)
	}
	_, err = service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: before.Path, UID: 1001, GID: 101, ExpectedRevision: before.Revision})
	if !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("stale owner = %v", err)
	}
	_, err = service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: "/srv/notes-link", UID: 1001, GID: 101, ExpectedRevision: "any"})
	if !errors.Is(err, sftp.ErrNotRegularFile) || len(remote.chownPaths) != 1 {
		t.Fatalf("link ownership = %v, writes = %v", err, remote.chownPaths)
	}
	if string(remote.nodes[before.Path].content) != "hello" {
		t.Fatal("ownership wrote contents")
	}
}

func TestOwnershipRejectsMissingAttributesPermissionsAndUnsupportedServers(t *testing.T) {
	for _, refusal := range []error{sftp.ErrOwnershipUnavailable, fs.ErrPermission, sftp.ErrUnsupportedOperation} {
		t.Run(refusal.Error(), func(t *testing.T) {
			remote := metadataFixture()
			service := metadataService(remote)
			if refusal == sftp.ErrOwnershipUnavailable {
				delete(remote.owners, "/srv/notes.txt")
			} else {
				remote.chownErr = refusal
			}
			before, err := service.Stat(t.Context(), "edge", "/srv/notes.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: before.Path, UID: 7, GID: 8, ExpectedRevision: before.Revision})
			if !errors.Is(err, refusal) || len(remote.chownPaths) != 0 {
				t.Fatalf("refused ownership = %v", err)
			}
		})
	}
	service := metadataService(remoteWith(linkedTree()))
	_, err := service.CreateSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: "/srv/new", Target: "absent"})
	if !errors.Is(err, sftp.ErrUnsupportedOperation) {
		t.Fatalf("unsupported symlink = %v", err)
	}
}

func TestOwnershipRejectsServerSuccessWithoutChangingTheRequestedIDs(t *testing.T) {
	remote := metadataFixture()
	remote.preserveOwnership = true
	service := metadataService(remote)
	before, err := service.Stat(t.Context(), "edge", "/srv/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: before.Path, UID: 7, GID: 8, ExpectedRevision: before.Revision})
	if !errors.Is(err, sftp.ErrUnsupportedOperation) || len(remote.chownPaths) != 1 {
		t.Fatalf("ignored ownership = %v, requests = %v", err, remote.chownPaths)
	}
}

func TestFilesystemSpaceUsesAvailableBlocksAndRejectsOverflow(t *testing.T) {
	tests := []struct {
		name             string
		stats            *pkgsftp.StatVFS
		err              error
		available, total uint64
	}{
		{name: "account available", stats: &pkgsftp.StatVFS{Frsize: 4096, Blocks: 10, Bfree: 9, Bavail: 3}, available: 12288, total: 40960},
		{name: "uint64 exact", stats: &pkgsftp.StatVFS{Frsize: 1, Blocks: math.MaxUint64, Bavail: math.MaxUint64}, available: math.MaxUint64, total: math.MaxUint64},
		{name: "multiply overflow", stats: &pkgsftp.StatVFS{Frsize: 2, Blocks: math.MaxUint64}, err: sftp.ErrInvalidSpace},
		{name: "available exceeds total", stats: &pkgsftp.StatVFS{Frsize: 1, Blocks: 1, Bavail: 2}, err: sftp.ErrInvalidSpace},
		{name: "zero fragment size", stats: &pkgsftp.StatVFS{}, err: sftp.ErrInvalidSpace},
		{name: "nil response", err: sftp.ErrInvalidSpace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := metadataFixture()
			remote.stats = test.stats
			space, err := metadataService(remote).FilesystemSpace(t.Context(), "edge", "/")
			if !errors.Is(err, test.err) || space.AvailableBytes != test.available || space.TotalBytes != test.total {
				t.Fatalf("space = %+v, %v", space, err)
			}
		})
	}
	_, err := metadataService(remoteWith(nil)).FilesystemSpace(t.Context(), "edge", "/")
	if !errors.Is(err, sftp.ErrUnsupportedOperation) {
		t.Fatalf("unsupported space = %v", err)
	}
}

func TestMetadataCapabilitiesSurvivePoolAndContextWrappers(t *testing.T) {
	remote := metadataFixture()
	remote.stats = &pkgsftp.StatVFS{Frsize: 1, Blocks: 3, Bavail: 2}
	pool := sftp.NewRemotePool(func(context.Context, string) (sftp.RemoteTarget, error) {
		return sftp.RemoteTarget{Identity: "fixture", Open: func(context.Context) (sftp.Remote, error) { return remote, nil }}, nil
	})
	defer pool.Close()
	service := sftp.Service{Open: pool.Open}
	if _, err := service.CreateSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: "/srv/new", Target: "absent"}); err != nil {
		t.Fatal(err)
	}
	link, err := service.Stat(t.Context(), "edge", "/srv/new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeSymlink(t.Context(), "edge", sftp.SymlinkChange{Path: link.Path, Target: "other", ExpectedRevision: link.Revision}); err != nil {
		t.Fatal(err)
	}
	entry, err := service.Stat(t.Context(), "edge", "/srv/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeOwnership(t.Context(), "edge", sftp.OwnershipChange{Path: entry.Path, UID: 2000, GID: 2000, ExpectedRevision: entry.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FilesystemSpace(t.Context(), "edge", "/srv"); err != nil {
		t.Fatal(err)
	}
}

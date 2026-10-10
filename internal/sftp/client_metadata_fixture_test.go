package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

type fixtureInfo struct{ name string }

func (info fixtureInfo) Name() string       { return info.name }
func (info fixtureInfo) Mode() fs.FileMode  { return 0o644 }
func (info fixtureInfo) Size() int64        { return 1 }
func (info fixtureInfo) ModTime() time.Time { return time.Unix(1, 0) }
func (info fixtureInfo) IsDir() bool        { return false }
func (info fixtureInfo) Sys() any           { return nil }

type fixtureOwnerInfo struct {
	fixtureInfo
	uid, gid uint32
}

func (info fixtureOwnerInfo) Uid() uint32 { return info.uid }
func (info fixtureOwnerInfo) Gid() uint32 { return info.gid }

type metadataLister []os.FileInfo

func (entries metadataLister) ListAt(destination []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(entries)) {
		return 0, io.EOF
	}
	count := copy(destination, entries[offset:])
	if int(offset)+count == len(entries) {
		return count, io.EOF
	}
	return count, nil
}
func (entries metadataLister) Filelist(request *pkgsftp.Request) (pkgsftp.ListerAt, error) {
	if request.Method == "List" {
		return entries, nil
	}
	for _, info := range entries {
		if request.Filepath == "/"+info.Name() {
			return metadataLister{info}, nil
		}
	}
	return nil, fs.ErrNotExist
}

func TestSFTPWireOwnersKeepMissingAttributesDistinctFromZeroAndSurviveListings(t *testing.T) {
	serverConnection, clientConnection := net.Pipe()
	server := pkgsftp.NewRequestServer(serverConnection, pkgsftp.Handlers{FileList: metadataLister{
		fixtureInfo{name: "missing"}, fixtureOwnerInfo{fixtureInfo: fixtureInfo{name: "zero"}}, fixtureOwnerInfo{fixtureInfo: fixtureInfo{name: "owned"}, uid: 1000, gid: 100},
	}})
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(); _ = server.Close() }()
	raw, err := pkgsftp.NewClientPipe(&attributeReader{source: clientConnection}, clientConnection)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(raw)
	defer func() { _ = client.Close(); <-done }()
	service := Service{Open: func(context.Context, string) (Remote, error) { return &fixtureRetainedClient{Client: client}, nil }}
	listing, err := service.ListDirectory(t.Context(), "fixture", "/")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if entry.Name == "missing" && entry.Ownership != nil {
			t.Fatalf("missing IDs became an owner: %+v", entry)
		}
		if entry.Name == "zero" && (entry.Ownership == nil || entry.Ownership.UID != 0 || entry.Ownership.GID != 0) {
			t.Fatalf("legitimate 0/0 lost: %+v", entry)
		}
		fromStat, err := service.Stat(t.Context(), "fixture", entry.Path)
		if err != nil || (fromStat.Ownership == nil) != (entry.Ownership == nil) || fromStat.Revision != entry.Revision {
			t.Fatalf("stat differs from list: %+v, %v", fromStat, err)
		}
	}
}

type fixtureRetainedClient struct{ *Client }

func (*fixtureRetainedClient) Close() error { return nil }

// The fixture refuses SETSTAT and link operations without exercising OS ownership.
type refusingMetadataCommands struct{}

func (refusingMetadataCommands) Filecmd(*pkgsftp.Request) error { return pkgsftp.ErrSSHFxOpUnsupported }

func TestUnsupportedProtocolResponsesAreReportedAsUnsupportedOperations(t *testing.T) {
	serverConnection, clientConnection := net.Pipe()
	server := pkgsftp.NewRequestServer(serverConnection, pkgsftp.Handlers{FileCmd: refusingMetadataCommands{}})
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(); _ = server.Close() }()
	raw, err := pkgsftp.NewClientPipe(clientConnection, clientConnection)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(raw)
	defer func() { _ = client.Close(); <-done }()
	operations := []func() error{
		func() error { return client.Chown("/file", 1, 2) },
		func() error { return client.Symlink("target", "/link") },
		func() error { return client.ReplaceSymlink("/temporary", "/link") },
		func() error { _, err := client.StatVFS("/"); return err },
	}
	for _, operation := range operations {
		if err := operation(); !errors.Is(err, ErrUnsupportedOperation) {
			t.Fatalf("unsupported protocol response = %v", err)
		}
	}
}

type incompleteMetadataInfo struct{ fixtureInfo }

func (incompleteMetadataInfo) Sys() any {
	return &pkgsftp.FileStat{UID: 1, GID: 2, Extended: []pkgsftp.StatExtended{{ExtType: metadataFlagsAttribute, ExtData: "2"}}}
}

type incompleteMetadataRemote struct {
	Remote
	writes int
}

func (*incompleteMetadataRemote) Close() error { return nil }
func (*incompleteMetadataRemote) Lstat(string) (fs.FileInfo, error) {
	return incompleteMetadataInfo{fixtureInfo: fixtureInfo{name: "unknown"}}, nil
}
func (remote *incompleteMetadataRemote) Chown(string, uint32, uint32) error {
	remote.writes++
	return nil
}

func TestMissingEntryTypeCannotAuthorizeOwnershipChangeEvenWithOwnerIDs(t *testing.T) {
	remote := &incompleteMetadataRemote{}
	service := Service{Open: func(context.Context, string) (Remote, error) { return remote, nil }}
	entry, err := service.Stat(t.Context(), "fixture", "/unknown")
	if err != nil || entry.Type != EntryOther || entry.Ownership == nil || entry.Ownership.UID != 1 {
		t.Fatalf("incomplete metadata = %+v, %v", entry, err)
	}
	_, err = service.ChangeOwnership(t.Context(), "fixture", OwnershipChange{Path: entry.Path, UID: 3, GID: 4, ExpectedRevision: entry.Revision})
	if !errors.Is(err, ErrMetadataUnavailable) || remote.writes != 0 {
		t.Fatalf("unknown type changed ownership: %v, writes %d", err, remote.writes)
	}
}

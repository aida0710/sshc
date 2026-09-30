package sftp_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/sftp"
)

// cancellableConnection is one SFTP connection opened for ctx. Cancelling ctx
// discards the connection (see bindRemoteContext), so a removal sent on it
// afterwards fails the way a request on a closed pkg/sftp client does.
type cancellableConnection struct {
	*fakeRemote
	ctx context.Context
}

func (c cancellableConnection) Close() error { return nil }
func (c cancellableConnection) Remove(candidate string) error {
	if c.ctx.Err() != nil {
		return net.ErrClosed
	}
	return c.fakeRemote.Remove(candidate)
}

func temporariesIn(remote *fakeRemote) []string {
	var found []string
	for candidate := range remote.nodes {
		if strings.Contains(candidate, ".sshc-") {
			found = append(found, candidate)
		}
	}
	return found
}

// cancelAfterFirstReport stops the transfer once its first bytes are written,
// as a pause or cancel from the transfer list does.
func cancelAfterFirstReport() (context.Context, func(int64) error) {
	ctx, cancel := context.WithCancel(context.Background())
	return ctx, func(int64) error {
		cancel()
		return nil
	}
}

// largerThanOneCopyChunk makes the transfer read more than one 2 MiB chunk,
// so the cancellation lands in the middle of the file.
const largerThanOneCopyChunk = 3 << 20

func TestCancelledPutRemovesItsTemporaryThroughANewConnection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	local := filepath.Join(home, "payload.bin")
	if err := os.WriteFile(local, make([]byte, largerThanOneCopyChunk), 0o600); err != nil {
		t.Fatal(err)
	}
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := sftp.Service{Open: func(ctx context.Context, _ string) (sftp.Remote, error) {
		return cancellableConnection{fakeRemote: target, ctx: ctx}, nil
	}}
	ctx, progress := cancelAfterFirstReport()
	err := service.CopyLocal(ctx, sftp.RemoteTransferRequest{
		SourcePath: filepath.ToSlash(local), TargetAlias: "edge", TargetPath: "/inbox/payload.bin", Operation: sftp.RemotePut,
	}, progress)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("put err = %v, want the cancellation", err)
	}
	if left := temporariesIn(target); len(left) != 0 {
		t.Fatalf("temporaries left behind: %v", left)
	}
}

func TestCancelledRemoteCopyRemovesItsTemporaryThroughANewConnection(t *testing.T) {
	source := remoteWith(map[string]node{"/payload.bin": {name: "payload.bin", mode: 0o644, content: make([]byte, largerThanOneCopyChunk), modTime: testTime}})
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := sftp.Service{Open: func(ctx context.Context, alias string) (sftp.Remote, error) {
		if alias == "source" {
			return fakeConnection{source}, nil
		}
		return cancellableConnection{fakeRemote: target, ctx: ctx}, nil
	}}
	ctx, progress := cancelAfterFirstReport()
	err := service.CopyRemote(ctx, sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/payload.bin", TargetAlias: "target", TargetPath: "/inbox/payload.bin",
		Operation: sftp.RemoteCopy,
	}, progress)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copy err = %v, want the cancellation", err)
	}
	if left := temporariesIn(target); len(left) != 0 {
		t.Fatalf("temporaries left behind: %v", left)
	}
}

const leftoverTemporary = "/work/.a.txt.sshc-0123456789abcdef01234567.tmp"

func workWithTemporaryModified(modified time.Time) *fakeRemote {
	return remoteWith(map[string]node{
		"/work":       directory("work"),
		"/work/a.txt": file("a.txt", "keep", 0o644),
		leftoverTemporary: {
			name: ".a.txt.sshc-0123456789abcdef01234567.tmp", mode: 0o600, content: []byte("partial"), modTime: modified,
		},
	})
}

func TestDeleteRemovesAnAbandonedTemporaryWithItsFolder(t *testing.T) {
	remote := workWithTemporaryModified(time.Now().Add(-48 * time.Hour))
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	if err := service.DeleteTreeForTest(context.Background(), "edge", "/work"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, exists := remote.nodes["/work"]; exists {
		t.Fatal("the folder is still there")
	}
}

func TestDeleteStillRefusesAFolderWhoseTemporaryIsBeingWritten(t *testing.T) {
	remote := workWithTemporaryModified(time.Now())
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{remote}, nil }}
	if err := service.DeleteTreeForTest(context.Background(), "edge", "/work"); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("delete err = %v, want a conflict", err)
	}
	if len(remote.removals) != 0 {
		t.Fatalf("removed %v before refusing", remote.removals)
	}
}

func TestMoveBetweenHostsLeavesAnAbandonedTemporaryBehindAndRemovesIt(t *testing.T) {
	source := workWithTemporaryModified(time.Now().Add(-48 * time.Hour))
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := twoHostService(fakeConnection{source}, fakeConnection{target})
	if err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/work", TargetAlias: "target", TargetPath: "/inbox/work",
		Operation: sftp.RemoteMove,
	}, nil); err != nil {
		t.Fatalf("move: %v", err)
	}
	if string(target.nodes["/inbox/work/a.txt"].content) != "keep" {
		t.Fatalf("target = %+v", target.nodes)
	}
	if left := temporariesIn(target); len(left) != 0 {
		t.Fatalf("the abandoned temporary was copied: %v", left)
	}
	if _, exists := source.nodes["/work"]; exists {
		t.Fatalf("source after the move = %+v", source.nodes)
	}
}

// sourceTimeOlderThanADay is a source modified long before the transfer, so a
// temporary carrying it would look abandoned to a delete or move of its folder.
var sourceTimeOlderThanADay = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// requireNoTemporaryCarriesTheSourceTimeAtPublish fails the test when a
// temporary in remote already has the source's time as it is renamed into place.
func requireNoTemporaryCarriesTheSourceTimeAtPublish(t *testing.T, remote *fakeRemote) {
	t.Helper()
	remote.renameHook = func() {
		for _, candidate := range temporariesIn(remote) {
			if remote.nodes[candidate].modTime.Equal(sourceTimeOlderThanADay) {
				t.Errorf("temporary %s had the source's time before it was published", candidate)
			}
		}
	}
}

func TestPutGivesTheSourceTimeToTheTargetOnlyAfterPublishing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	local := filepath.Join(home, "payload.bin")
	if err := os.WriteFile(local, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(local, sourceTimeOlderThanADay, sourceTimeOlderThanADay); err != nil {
		t.Fatal(err)
	}
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	requireNoTemporaryCarriesTheSourceTimeAtPublish(t, target)
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{target}, nil }}
	if err := service.CopyLocal(context.Background(), sftp.RemoteTransferRequest{
		SourcePath: filepath.ToSlash(local), TargetAlias: "edge", TargetPath: "/inbox/payload.bin", Operation: sftp.RemotePut,
	}, nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	if got := target.nodes["/inbox/payload.bin"].modTime; !got.Equal(sourceTimeOlderThanADay) {
		t.Fatalf("target time = %v, want the source's %v", got, sourceTimeOlderThanADay)
	}
}

func TestRemoteCopyGivesTheSourceTimeToTheTargetOnlyAfterPublishing(t *testing.T) {
	source := remoteWith(map[string]node{"/payload.bin": {name: "payload.bin", mode: 0o644, content: []byte("payload"), modTime: sourceTimeOlderThanADay}})
	target := remoteWith(map[string]node{"/inbox": directory("inbox")})
	requireNoTemporaryCarriesTheSourceTimeAtPublish(t, target)
	service := twoHostService(fakeConnection{source}, fakeConnection{target})
	if err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/payload.bin", TargetAlias: "target", TargetPath: "/inbox/payload.bin",
		Operation: sftp.RemoteCopy,
	}, nil); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := target.nodes["/inbox/payload.bin"].modTime; !got.Equal(sourceTimeOlderThanADay) {
		t.Fatalf("target time = %v, want the source's %v", got, sourceTimeOlderThanADay)
	}
}

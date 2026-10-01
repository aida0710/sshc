package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

// serverWithSourceFolder holds /work/source with one file, the source of every
// transfer below, and the extra entries.
func serverWithSourceFolder(extra map[string]node) *fakeRemote {
	nodes := map[string]node{
		"/work":                 directory("work"),
		"/work/source":          directory("source"),
		"/work/source/file.txt": file("file.txt", "contents", 0o644),
	}
	maps.Copy(nodes, extra)
	return remoteWith(nodes)
}

// oneServer answers every alias with the same server, as web and web-admin do
// when both name one host.
func oneServer(server sftp.Remote) sftp.Service {
	return sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return server, nil }}
}

// requireUnchanged fails when a refused transfer added or removed an entry,
// including a probe file it did not clean up.
func requireUnchanged(t *testing.T, server *fakeRemote, original []string) {
	t.Helper()
	current := nodeNames(server)
	if !slices.Equal(current, original) {
		t.Fatalf("server holds %v, want only %v", current, original)
	}
}

// nodeNames lists the server's paths in order.
func nodeNames(server *fakeRemote) []string {
	names := make([]string, 0, len(server.nodes))
	for name := range server.nodes {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

var folderOperations = []sftp.RemoteTransferOperation{sftp.RemoteCopy, sftp.RemoteMove}

func TestRemoteDirectoryCannotBeCopiedIntoItself(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodeNames(server)
	service := oneServer(server)

	for _, operation := range folderOperations {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "same", SourcePath: "/work/source",
			TargetAlias: "same", TargetPath: "/work/source/nested", Operation: operation,
		}, nil)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(%s) error = %v, want ErrTargetInsideSource", operation, err)
		}
	}
	requireUnchanged(t, server, original)
}

func TestRemoteDirectoryCannotBeCopiedIntoItselfThroughAnotherAliasOfTheSameServer(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodeNames(server)
	service := oneServer(server)

	for _, operation := range folderOperations {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "web", SourcePath: "/work/source",
			TargetAlias: "web-admin", TargetPath: "/work/source/nested", Operation: operation,
		}, nil)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(%s) error = %v, want ErrTargetInsideSource", operation, err)
		}
	}
	requireUnchanged(t, server, original)
}

func TestRemoteDirectoryCannotBeCopiedIntoItselfThroughASymlinkOnTheTargetPath(t *testing.T) {
	t.Parallel()
	for _, aliases := range [][2]string{{"same", "same"}, {"web", "web-admin"}} {
		server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work/source")})
		original := nodeNames(server)
		service := oneServer(server)
		for _, operation := range folderOperations {
			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: aliases[0], SourcePath: "/work/source",
				TargetAlias: aliases[1], TargetPath: "/shortcut/nested", Operation: operation,
			}, nil)
			if !errors.Is(err, sftp.ErrTargetInsideSource) {
				t.Fatalf("CopyRemote(%s, %s→%s) error = %v, want ErrTargetInsideSource", operation, aliases[0], aliases[1], err)
			}
		}
		requireUnchanged(t, server, original)
	}
}

func TestRemoteDirectoryCannotBeCopiedIntoItselfWhenTheSourcePathGoesThroughASymlink(t *testing.T) {
	t.Parallel()
	// Only RealPath of the fake follows links, so the folder is also listed
	// under the link's path, as a server shows it there.
	server := serverWithSourceFolder(map[string]node{
		"/link":                 symlink("link", "work"),
		"/link/source":          directory("source"),
		"/link/source/file.txt": file("file.txt", "contents", 0o644),
	})
	original := nodeNames(server)
	service := oneServer(server)

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "same", SourcePath: "/link/source",
		TargetAlias: "same", TargetPath: "/work/source/nested", Operation: sftp.RemoteCopy,
	}, nil)
	if !errors.Is(err, sftp.ErrTargetInsideSource) {
		t.Fatalf("CopyRemote() error = %v, want ErrTargetInsideSource", err)
	}
	requireUnchanged(t, server, original)
}

// A move onto the source folder through another alias would copy every file
// onto itself and then delete the source, which is the only copy.
func TestRemoteDirectoryCannotBeMovedOntoItselfThroughAnotherAliasOfTheSameServer(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work")})
	original := nodeNames(server)
	service := oneServer(server)

	for _, targetPath := range []string{"/work/source", "/shortcut/source"} {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "web", SourcePath: "/work/source",
			TargetAlias: "web-admin", TargetPath: targetPath,
			Operation: sftp.RemoteMove, Overwrite: true,
		}, nil)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(move to %s) error = %v, want ErrTargetInsideSource", targetPath, err)
		}
	}
	requireUnchanged(t, server, original)
}

func TestRemoteDirectoryCanBeCopiedNextToAFolderWhoseNameStartsTheSame(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	service := oneServer(server)

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "same", SourcePath: "/work/source",
		TargetAlias: "same", TargetPath: "/work/source-copy", Operation: sftp.RemoteCopy,
	}, nil)
	if err != nil {
		t.Fatalf("CopyRemote() error = %v", err)
	}
	if got := string(server.nodes["/work/source-copy/file.txt"].content); got != "contents" {
		t.Fatalf("copied file = %q, want contents", got)
	}
}

func TestRemoteDirectoryCanBeCopiedBelowTheSamePathOnAnotherServer(t *testing.T) {
	t.Parallel()
	sourceServer := serverWithSourceFolder(nil)
	targetServer := remoteWith(map[string]node{
		"/work":        directory("work"),
		"/work/source": directory("source"),
	})
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "origin" {
			return sourceServer, nil
		}
		return targetServer, nil
	}}

	for _, targetPath := range []string{"/work/source/nested", "/work/source"} {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "origin", SourcePath: "/work/source",
			TargetAlias: "mirror", TargetPath: targetPath, Operation: sftp.RemoteCopy, Overwrite: true,
		}, nil)
		if err != nil {
			t.Fatalf("CopyRemote(to %s) error = %v", targetPath, err)
		}
		if got := string(targetServer.nodes[targetPath+"/file.txt"].content); got != "contents" {
			t.Fatalf("copied file at %s = %q, want contents", targetPath, got)
		}
	}
	for candidate := range targetServer.nodes {
		if strings.Contains(candidate, ".sshc-") {
			t.Fatalf("copy left %s on the target server", candidate)
		}
	}
}

// realPathAnswer is a server whose REALPATH answers one fixed value.
type realPathAnswer struct {
	*fakeRemote
	answer string
}

func (remote realPathAnswer) RealPath(string) (string, error) { return remote.answer, nil }

func TestCopyComparesTheRequestedPathsWhenTheServerCannotResolveThem(t *testing.T) {
	t.Parallel()
	unresolvable := map[string]func(*fakeRemote) sftp.Remote{
		"realpath fails": func(server *fakeRemote) sftp.Remote {
			server.realPathErr = fs.ErrPermission
			return server
		},
		"realpath answers a relative path": func(server *fakeRemote) sftp.Remote {
			return realPathAnswer{fakeRemote: server, answer: "work/elsewhere"}
		},
	}
	for name, unresolved := range unresolvable {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := serverWithSourceFolder(nil)
			service := oneServer(unresolved(server))

			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: "same", SourcePath: "/work/source",
				TargetAlias: "same", TargetPath: "/work/source/nested", Operation: sftp.RemoteCopy,
			}, nil)
			if !errors.Is(err, sftp.ErrTargetInsideSource) {
				t.Fatalf("copy into the source = %v, want ErrTargetInsideSource", err)
			}
			if err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: "same", SourcePath: "/work/source",
				TargetAlias: "same", TargetPath: "/work/source-copy", Operation: sftp.RemoteCopy,
			}, nil); err != nil {
				t.Fatalf("copy beside the source = %v, want it to run", err)
			}
		})
	}
}

func TestQueuedCopyIntoItselfFailsWithItsOwnProblemAndWritesNothing(t *testing.T) {
	server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work/source")})
	original := nodeNames(server)
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{server}, nil }}
	manager := newTestTransferManager(t, service)
	defer manager.Close()
	const id = "copy_into_itself"
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "edge", RemotePath: "/shortcut/nested",
		SourceAlias: "edge", SourcePath: "/work/source", Operation: sftp.RemoteCopy,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "source", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}

	job := waitForJob(t, manager, id, hasStatus(sftp.TransferFailed))
	if job.Problem != "sftp_target_inside_source" {
		t.Fatalf("problem = %q, want sftp_target_inside_source", job.Problem)
	}
	requireUnchanged(t, server, original)
}

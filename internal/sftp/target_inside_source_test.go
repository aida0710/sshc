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
	current := nodePaths(server)
	if !slices.Equal(current, original) {
		t.Fatalf("server holds %v, want only %v", current, original)
	}
}

// nodePaths lists the server's paths in order.
func nodePaths(server *fakeRemote) []string {
	paths := make([]string, 0, len(server.nodes))
	for candidate := range server.nodes {
		paths = append(paths, candidate)
	}
	slices.Sort(paths)
	return paths
}

// transferAliases names the two ends of a transfer.
type transferAliases struct{ source, target string }

var (
	folderOperations = []sftp.RemoteTransferOperation{sftp.RemoteCopy, sftp.RemoteMove}
	// oneAliasAndTwoAliases reach one server through the same alias and through
	// two aliases, as web and web-admin both naming one host.
	oneAliasAndTwoAliases = []transferAliases{{source: "same", target: "same"}, {source: "web", target: "web-admin"}}
)

// fixedRealPath is a connection whose REALPATH always gives the same answer,
// as a server does that cannot resolve the paths or answers oddly.
type fixedRealPath struct {
	*fakeRemote
	answer string
	err    error
}

func (remote fixedRealPath) RealPath(string) (string, error) { return remote.answer, remote.err }

// errCopyStarted ends a copy that the check let through at its first written
// bytes, so that a regression fails at once instead of nesting the source into
// itself until the tree limit.
var errCopyStarted = errors.New("the copy started writing")

func stopAtFirstBytes(int64) error { return errCopyStarted }

func TestRemoteFolderCopyOrMoveIntoItselfIsRefused(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodePaths(server)
	service := oneServer(server)

	for _, operation := range folderOperations {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "same", SourcePath: "/work/source",
			TargetAlias: "same", TargetPath: "/work/source/nested", Operation: operation,
		}, stopAtFirstBytes)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(%s) error = %v, want ErrTargetInsideSource", operation, err)
		}
	}
	requireUnchanged(t, server, original)
}

// Approving an overwrite cannot make a folder its own destination, so the
// transfer is refused instead of asking for an approval that would not help.
func TestRemoteFolderCopyOrMoveOntoItselfOnOneAliasIsRefusedInsteadOfAskingToOverwrite(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodePaths(server)
	service := oneServer(server)

	for _, operation := range folderOperations {
		for _, overwrite := range []bool{false, true} {
			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: "same", SourcePath: "/work/source",
				TargetAlias: "same", TargetPath: "/work/source", Operation: operation, Overwrite: overwrite,
			}, stopAtFirstBytes)
			if !errors.Is(err, sftp.ErrTargetInsideSource) {
				t.Fatalf("CopyRemote(%s, overwrite %v) error = %v, want ErrTargetInsideSource", operation, overwrite, err)
			}
		}
	}
	requireUnchanged(t, server, original)
}

func TestRemoteFolderCopyOrMoveIntoItselfIsRefusedThroughAnotherAliasOfTheSameServer(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodePaths(server)
	service := oneServer(server)

	for _, operation := range folderOperations {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "web", SourcePath: "/work/source",
			TargetAlias: "web-admin", TargetPath: "/work/source/nested", Operation: operation,
		}, stopAtFirstBytes)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(%s) error = %v, want ErrTargetInsideSource", operation, err)
		}
	}
	requireUnchanged(t, server, original)
}

func TestRemoteFolderCopyOrMoveIntoItselfIsRefusedThroughASymlinkOnTheTargetPath(t *testing.T) {
	t.Parallel()
	for _, aliases := range oneAliasAndTwoAliases {
		server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work/source")})
		original := nodePaths(server)
		service := oneServer(server)
		for _, operation := range folderOperations {
			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: aliases.source, SourcePath: "/work/source",
				TargetAlias: aliases.target, TargetPath: "/shortcut/nested", Operation: operation,
			}, stopAtFirstBytes)
			if !errors.Is(err, sftp.ErrTargetInsideSource) {
				t.Fatalf("CopyRemote(%s, %s→%s) error = %v, want ErrTargetInsideSource", operation, aliases.source, aliases.target, err)
			}
		}
		requireUnchanged(t, server, original)
	}
}

// serverWithSourceFolderBehindALink adds /link, a link to /work. Only RealPath
// of the fake follows links, so the folder is also listed under the link's
// path, as a server shows it there.
func serverWithSourceFolderBehindALink() *fakeRemote {
	return serverWithSourceFolder(map[string]node{
		"/link":                 symlink("link", "work"),
		"/link/source":          directory("source"),
		"/link/source/file.txt": file("file.txt", "contents", 0o644),
	})
}

func TestRemoteFolderCopyIntoItselfIsRefusedWhenTheSourcePathGoesThroughASymlink(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolderBehindALink()
	original := nodePaths(server)
	service := oneServer(server)

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "same", SourcePath: "/link/source",
		TargetAlias: "same", TargetPath: "/work/source/nested", Operation: sftp.RemoteCopy,
	}, stopAtFirstBytes)
	if !errors.Is(err, sftp.ErrTargetInsideSource) {
		t.Fatalf("CopyRemote() error = %v, want ErrTargetInsideSource", err)
	}
	requireUnchanged(t, server, original)
}

// A move onto the source folder through another alias would copy every file
// onto itself and then delete the source, which is the only copy.
func TestRemoteFolderMoveOntoItselfIsRefusedThroughAnotherAliasOfTheSameServer(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work")})
	original := nodePaths(server)
	service := oneServer(server)

	for _, targetPath := range []string{"/work/source", "/shortcut/source"} {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "web", SourcePath: "/work/source",
			TargetAlias: "web-admin", TargetPath: targetPath,
			Operation: sftp.RemoteMove, Overwrite: true,
		}, stopAtFirstBytes)
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("CopyRemote(move to %s) error = %v, want ErrTargetInsideSource", targetPath, err)
		}
	}
	requireUnchanged(t, server, original)
}

// Telling whether another alias reaches the same server writes a probe file.
// Before the overwrite is approved, nothing is written into the existing
// folder, whichever server it is on.
func TestRemoteFolderCopyOntoAnExistingFolderOfAnotherAliasAsksToOverwriteBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	targets := map[string]func(source *fakeRemote) *fakeRemote{
		"another server": func(*fakeRemote) *fakeRemote {
			return remoteWith(map[string]node{"/work": directory("work"), "/work/source": directory("source")})
		},
		"the same server": func(source *fakeRemote) *fakeRemote { return source },
	}
	for name, targetOf := range targets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sourceServer := serverWithSourceFolder(nil)
			targetServer := targetOf(sourceServer)
			created := 0
			targetServer.createHook = func() { created++ }
			service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
				if alias == "origin" {
					return sourceServer, nil
				}
				return targetServer, nil
			}}

			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: "origin", SourcePath: "/work/source",
				TargetAlias: "mirror", TargetPath: "/work/source", Operation: sftp.RemoteCopy,
			}, nil)
			if !errors.Is(err, sftp.ErrAlreadyExists) {
				t.Fatalf("CopyRemote() error = %v, want ErrAlreadyExists", err)
			}
			if created != 0 {
				t.Fatalf("the target got %d files before the overwrite was approved", created)
			}
		})
	}
}

func TestRemoteFolderCanBeCopiedNextToAFolderWhoseNameStartsTheSame(t *testing.T) {
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

func TestRemoteFolderCanBeCopiedBelowTheSamePathOnAnotherServer(t *testing.T) {
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

// A server that cannot resolve the paths does not make the transfer fail as
// one into itself: the requested paths decide, so a copy into the source is
// still refused and a copy beside it runs.
func TestCopyIntoItselfIsStillRefusedAndBesideItAllowedWhenTheServerCannotResolvePaths(t *testing.T) {
	t.Parallel()
	unresolvable := map[string]func(*fakeRemote) sftp.Remote{
		"realpath fails": func(server *fakeRemote) sftp.Remote {
			return fixedRealPath{fakeRemote: server, err: fs.ErrPermission}
		},
		"realpath answers a relative path": func(server *fakeRemote) sftp.Remote {
			return fixedRealPath{fakeRemote: server, answer: "work/elsewhere"}
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
			}, stopAtFirstBytes)
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

// When only the source's connection resolves its path, comparing that
// resolved path with the target's requested one would compare one place under
// two names and miss it, so both requested paths are compared instead.
func TestCopyIntoItselfIsRefusedWhenOnlyTheSourceConnectionCanResolvePaths(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolderBehindALink()
	original := nodePaths(server)
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		if alias == "web" {
			return server, nil
		}
		return fixedRealPath{fakeRemote: server, err: fs.ErrPermission}, nil
	}}

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "web", SourcePath: "/link/source",
		TargetAlias: "web-admin", TargetPath: "/link/source/nested", Operation: sftp.RemoteCopy,
	}, stopAtFirstBytes)
	if !errors.Is(err, sftp.ErrTargetInsideSource) {
		t.Fatalf("CopyRemote() error = %v, want ErrTargetInsideSource", err)
	}
	requireUnchanged(t, server, original)
}

// On one alias the refusal only reads, so planning refuses the transfer
// before it lists the source tree to count its size.
func TestPlanningAFolderTransferIntoItselfOnOneAliasIsRefusedBeforeCountingTheTree(t *testing.T) {
	t.Parallel()
	for _, targetPath := range []string{"/work/source/nested", "/work/source", "/shortcut/nested"} {
		server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work/source")})
		var listed []string
		server.readDirHook = func(directory string) error {
			listed = append(listed, directory)
			return nil
		}

		_, err := oneServer(server).PlanRemoteTransfer(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "same", SourcePath: "/work/source",
			TargetAlias: "same", TargetPath: targetPath, Operation: sftp.RemoteCopy,
		})
		if !errors.Is(err, sftp.ErrTargetInsideSource) {
			t.Fatalf("PlanRemoteTransfer(to %s) error = %v, want ErrTargetInsideSource", targetPath, err)
		}
		if len(listed) != 0 {
			t.Fatalf("planning the transfer to %s listed %v before refusing it", targetPath, listed)
		}
	}
}

func TestQueuedCopyIntoItselfFailsWithItsOwnProblemAndWritesNothing(t *testing.T) {
	server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work/source")})
	original := nodePaths(server)
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{server}, nil }}
	manager := newTestTransferManager(t, service)
	defer manager.Close()

	for _, aliases := range oneAliasAndTwoAliases {
		id := "copy_into_itself_to_" + aliases.target
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_" + id, Alias: aliases.target, RemotePath: "/shortcut/nested",
			SourceAlias: aliases.source, SourcePath: "/work/source", Operation: sftp.RemoteCopy,
			Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, Name: "source", TotalBytes: -1,
		}); err != nil {
			t.Fatal(err)
		}

		job := waitForJob(t, manager, id, hasStatus(sftp.TransferFailed))
		if job.Problem != "sftp_target_inside_source" {
			t.Fatalf("problem of the job from %s to %s = %q, want sftp_target_inside_source", aliases.source, aliases.target, job.Problem)
		}
	}
	requireUnchanged(t, server, original)
}

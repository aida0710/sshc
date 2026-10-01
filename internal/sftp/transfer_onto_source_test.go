package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

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
	copyAndMove = []sftp.RemoteTransferOperation{sftp.RemoteCopy, sftp.RemoteMove}
	// oneAliasAndTwoAliases reach one server through the same alias and through
	// two aliases, as web and web-admin both naming one host.
	oneAliasAndTwoAliases = []transferAliases{{source: "same", target: "same"}, {source: "web", target: "web-admin"}}
)

// sourceEntry is the source folder of serverWithSourceFolder or the file in
// it, with the refusal of a transfer onto itself.
type sourceEntry struct {
	kind, path string
	refusal    error
}

var sourceFolderAndFile = []sourceEntry{
	{kind: "folder", path: "/work/source", refusal: sftp.ErrTargetInsideSource},
	{kind: "file", path: "/work/source/file.txt", refusal: sftp.ErrTargetIsSource},
}

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

	for _, operation := range copyAndMove {
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

// Approving an overwrite cannot make a folder or a file its own destination,
// so planning and running refuse the transfer instead of asking for an
// approval that would not help.
func TestRemoteCopyOrMoveOntoItselfOnOneAliasIsRefusedInsteadOfAskingToOverwrite(t *testing.T) {
	t.Parallel()
	for _, source := range sourceFolderAndFile {
		server := serverWithSourceFolder(nil)
		original := nodePaths(server)
		service := oneServer(server)
		for _, operation := range copyAndMove {
			for _, overwrite := range []bool{false, true} {
				request := sftp.RemoteTransferRequest{
					SourceAlias: "same", SourcePath: source.path,
					TargetAlias: "same", TargetPath: source.path, Operation: operation, Overwrite: overwrite,
				}
				if _, err := service.PlanRemoteTransfer(context.Background(), request); !errors.Is(err, source.refusal) {
					t.Fatalf("PlanRemoteTransfer(%s %s, overwrite %v) error = %v, want %v", operation, source.kind, overwrite, err, source.refusal)
				}
				if err := service.CopyRemote(context.Background(), request, stopAtFirstBytes); !errors.Is(err, source.refusal) {
					t.Fatalf("CopyRemote(%s %s, overwrite %v) error = %v, want %v", operation, source.kind, overwrite, err, source.refusal)
				}
			}
		}
		requireUnchanged(t, server, original)
	}
}

func TestRemoteFolderCopyOrMoveIntoItselfIsRefusedThroughAnotherAliasOfTheSameServer(t *testing.T) {
	t.Parallel()
	server := serverWithSourceFolder(nil)
	original := nodePaths(server)
	service := oneServer(server)

	for _, operation := range copyAndMove {
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
		for _, operation := range copyAndMove {
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

// A move onto the source through another alias of the same server would copy
// each file onto itself and then delete the source, which is the only copy.
// The same holds for a destination reached through a link to a folder on the
// way, also on one alias, where the copy would only rewrite the source.
func TestRemoteCopyOrMoveOntoItselfWithTheOverwriteApprovedIsRefusedAndKeepsTheSource(t *testing.T) {
	t.Parallel()
	for _, source := range sourceFolderAndFile {
		for _, aliases := range oneAliasAndTwoAliases {
			server := serverWithSourceFolder(map[string]node{"/shortcut": symlink("shortcut", "/work")})
			original := nodePaths(server)
			service := oneServer(server)
			throughTheLink := "/shortcut" + strings.TrimPrefix(source.path, "/work")
			for _, targetPath := range []string{source.path, throughTheLink} {
				for _, operation := range copyAndMove {
					err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
						SourceAlias: aliases.source, SourcePath: source.path,
						TargetAlias: aliases.target, TargetPath: targetPath,
						Operation: operation, Overwrite: true,
					}, stopAtFirstBytes)
					if !errors.Is(err, source.refusal) {
						t.Fatalf("CopyRemote(%s %s to %s, %s→%s) error = %v, want %v", operation, source.kind, targetPath, aliases.source, aliases.target, err, source.refusal)
					}
				}
			}
			requireUnchanged(t, server, original)
			if got := string(server.nodes["/work/source/file.txt"].content); got != "contents" {
				t.Fatalf("the source file holds %q after the refused transfers, want contents", got)
			}
		}
	}
}

// Telling whether another alias reaches the same server writes a probe file.
// Before the overwrite is approved, nothing is written next to or into the
// existing folder or file, whichever server it is on.
func TestRemoteCopyOrMoveOntoAnExistingEntryOfAnotherAliasAsksToOverwriteBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	targets := map[string]func(source *fakeRemote) *fakeRemote{
		"another server":  func(*fakeRemote) *fakeRemote { return serverWithSourceFolder(nil) },
		"the same server": func(source *fakeRemote) *fakeRemote { return source },
	}
	for name, targetOf := range targets {
		for _, source := range sourceFolderAndFile {
			for _, operation := range copyAndMove {
				t.Run(name+"/"+string(operation)+" "+source.kind, func(t *testing.T) {
					t.Parallel()
					sourceServer := serverWithSourceFolder(nil)
					targetServer := targetOf(sourceServer)
					created := 0
					targetServer.createHook = func() { created++ }
					service := twoHostService(sourceServer, targetServer)

					err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
						SourceAlias: "source", SourcePath: source.path,
						TargetAlias: "target", TargetPath: source.path, Operation: operation,
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
	}
}

// firstLstatFails is a connection whose first Lstat of path fails, as a
// momentary permission or connection problem does, and whose later ones
// answer.
type firstLstatFails struct {
	*fakeRemote
	path   string
	failed bool
}

var errLstatFailed = errors.New("lstat failed")

func (remote *firstLstatFails) Lstat(candidate string) (fs.FileInfo, error) {
	if candidate == remote.path && !remote.failed {
		remote.failed = true
		return nil, errLstatFailed
	}
	return remote.fakeRemote.Lstat(candidate)
}

// Taking a target that could not be checked for a missing one would write a
// probe file before the overwrite is approved, or let a move onto the source
// itself through, which deletes the source once the copy finds the file.
func TestRemoteCopyOrMoveThatCannotCheckTheTargetStopsWithThatFailureAndWritesNothing(t *testing.T) {
	t.Parallel()
	for _, source := range sourceFolderAndFile {
		for _, operation := range copyAndMove {
			t.Run(string(operation)+" "+source.kind, func(t *testing.T) {
				t.Parallel()
				server := serverWithSourceFolder(nil)
				original := nodePaths(server)
				created := 0
				server.createHook = func() { created++ }
				service := twoHostService(server, &firstLstatFails{fakeRemote: server, path: source.path})

				err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
					SourceAlias: "source", SourcePath: source.path,
					TargetAlias: "target", TargetPath: source.path, Operation: operation, Overwrite: true,
				}, stopAtFirstBytes)
				if !errors.Is(err, errLstatFailed) {
					t.Fatalf("CopyRemote() error = %v, want the failure to check the target", err)
				}
				if created != 0 {
					t.Fatalf("the transfer wrote %d files after it could not check the target", created)
				}
				requireUnchanged(t, server, original)
			})
		}
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
	service := twoHostService(sourceServer, targetServer)

	for _, targetPath := range []string{"/work/source/nested", "/work/source"} {
		err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
			SourceAlias: "source", SourcePath: "/work/source",
			TargetAlias: "target", TargetPath: targetPath, Operation: sftp.RemoteCopy, Overwrite: true,
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

// On another server the same path names another file, which an approved copy
// or move replaces. Only a file that looks the same from both connections
// could be the source itself, so only then does telling the servers apart
// write a probe file, which it removes again.
func TestRemoteFileCanBeCopiedOrMovedToTheSamePathOnAnotherServer(t *testing.T) {
	t.Parallel()
	const sourceFile = "/work/source/file.txt"
	folders := map[string]node{"/work": directory("work"), "/work/source": directory("source")}
	foldersWithFile := func(existing node) map[string]node {
		nodes := maps.Clone(folders)
		nodes[sourceFile] = existing
		return nodes
	}
	targets := []struct {
		name  string
		nodes map[string]node
		// probes is how many probe files telling the servers apart writes.
		probes int
	}{
		{name: "onto a file of another size", nodes: foldersWithFile(file("file.txt", "older contents", 0o644))},
		{name: "onto a file of another time", nodes: foldersWithFile(withModTime(file("file.txt", "contents", 0o644), testTime.Add(time.Hour)))},
		{name: "onto a file of the same size and time", nodes: foldersWithFile(file("file.txt", "CONTENTS", 0o644)), probes: 1},
		{name: "as a new file", nodes: folders},
	}
	for _, target := range targets {
		for _, operation := range copyAndMove {
			t.Run(target.name+"/"+string(operation), func(t *testing.T) {
				t.Parallel()
				sourceServer := serverWithSourceFolder(nil)
				targetServer := remoteWith(maps.Clone(target.nodes))

				err := twoHostService(sourceServer, targetServer).CopyRemote(context.Background(), sftp.RemoteTransferRequest{
					SourceAlias: "source", SourcePath: sourceFile,
					TargetAlias: "target", TargetPath: sourceFile, Operation: operation, Overwrite: true,
				}, nil)
				if err != nil {
					t.Fatalf("CopyRemote() error = %v", err)
				}
				if got := string(targetServer.nodes[sourceFile].content); got != "contents" {
					t.Fatalf("the target file holds %q, want contents", got)
				}
				_, sourceKept := sourceServer.nodes[sourceFile]
				if sourceKept != (operation == sftp.RemoteCopy) {
					t.Fatalf("after the %s the source file is kept: %v", operation, sourceKept)
				}
				if removed := probeRemovals(targetServer); len(removed) != target.probes {
					t.Fatalf("the %s wrote and removed the probe files %v on the target server, want %d", operation, removed, target.probes)
				}
				requireNoProbeLeft(t, targetServer)
			})
		}
	}
}

// withModTime gives entry another modification time.
func withModTime(entry node, modified time.Time) node {
	entry.modTime = modified
	return entry
}

// probeRemovals lists the probe files removed from server. The check removes
// each probe file right after writing it, so this also counts those written.
func probeRemovals(server *fakeRemote) []string {
	return slices.DeleteFunc(slices.Clone(server.removals), func(removed string) bool {
		return !isProbeName(path.Base(removed))
	})
}

// requireNoProbeLeft fails when a probe file stayed on server.
func requireNoProbeLeft(t *testing.T, server *fakeRemote) {
	t.Helper()
	for candidate := range server.nodes {
		if isProbeName(path.Base(candidate)) {
			t.Fatalf("the check left the probe file %s", candidate)
		}
	}
}

// isProbeName is whether name is that of a probe file, which the check names
// as a temporary for sshc-probe.
func isProbeName(name string) bool { return strings.HasPrefix(name, ".sshc-probe.") }

// The destination's own link is not followed: like any other entry there, an
// approved copy or move replaces the link with the contents. The file the link
// pointed at is not overwritten; a copy keeps it and a move removes it as any
// move removes its source.
func TestRemoteFileCopyOrMoveOntoALinkToItselfReplacesTheLink(t *testing.T) {
	t.Parallel()
	for _, aliases := range oneAliasAndTwoAliases {
		for _, operation := range copyAndMove {
			server := serverWithSourceFolder(map[string]node{"/work/link.txt": symlink("link.txt", "/work/source/file.txt")})
			service := oneServer(server)

			err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
				SourceAlias: aliases.source, SourcePath: "/work/source/file.txt",
				TargetAlias: aliases.target, TargetPath: "/work/link.txt", Operation: operation, Overwrite: true,
			}, nil)
			if err != nil {
				t.Fatalf("CopyRemote(%s, %s→%s) error = %v", operation, aliases.source, aliases.target, err)
			}
			replaced := server.nodes["/work/link.txt"]
			if !replaced.mode.IsRegular() || string(replaced.content) != "contents" {
				t.Fatalf("after the %s from %s to %s the link is %v holding %q, want a file holding contents", operation, aliases.source, aliases.target, replaced.mode, replaced.content)
			}
			_, sourceKept := server.nodes["/work/source/file.txt"]
			if sourceKept != (operation == sftp.RemoteCopy) {
				t.Fatalf("after the %s from %s to %s the source file is kept: %v", operation, aliases.source, aliases.target, sourceKept)
			}
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
	service := twoHostService(server, fixedRealPath{fakeRemote: server, err: fs.ErrPermission})

	err := service.CopyRemote(context.Background(), sftp.RemoteTransferRequest{
		SourceAlias: "source", SourcePath: "/link/source",
		TargetAlias: "target", TargetPath: "/link/source/nested", Operation: sftp.RemoteCopy,
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

// Through another alias the job first asks to overwrite the existing file, as
// it cannot tell the server apart without writing a probe there. Once the
// overwrite is approved it fails with its own problem and keeps the file. On
// one alias it fails at once.
func TestQueuedFileMoveOntoItselfFailsWithItsOwnProblemAndKeepsTheFile(t *testing.T) {
	server := serverWithSourceFolder(nil)
	original := nodePaths(server)
	service := &sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return fakeConnection{server}, nil }}
	manager := newTestTransferManager(t, service)
	defer manager.Close()

	for _, aliases := range oneAliasAndTwoAliases {
		id := "move_onto_itself_to_" + aliases.target
		if _, err := manager.CreateJob(sftp.CreateTransferJob{
			ID: id, BatchID: "batch_" + id, Alias: aliases.target, RemotePath: "/work/source/file.txt",
			SourceAlias: aliases.source, SourcePath: "/work/source/file.txt", Operation: sftp.RemoteMove,
			Direction: sftp.TransferRemote, Kind: sftp.TransferFile, Name: "file.txt", TotalBytes: -1,
		}); err != nil {
			t.Fatal(err)
		}

		job := waitForJob(t, manager, id, func(job sftp.TransferJob) bool {
			return job.Status == sftp.TransferFailed || job.Status == sftp.TransferNeedsOverwrite
		})
		askedToOverwrite := job.Status == sftp.TransferNeedsOverwrite
		if askedToOverwrite != (aliases.source != aliases.target) {
			t.Fatalf("the move from %s to %s asked to overwrite: %v", aliases.source, aliases.target, askedToOverwrite)
		}
		if askedToOverwrite {
			if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); err != nil {
				t.Fatal(err)
			}
			job = waitForJob(t, manager, id, hasStatus(sftp.TransferFailed))
		}
		if job.Problem != "sftp_target_is_source" {
			t.Fatalf("problem of the move from %s to %s = %q, want sftp_target_is_source", aliases.source, aliases.target, job.Problem)
		}
	}
	requireUnchanged(t, server, original)
	if got := string(server.nodes["/work/source/file.txt"].content); got != "contents" {
		t.Fatalf("the source file holds %q after the refused moves, want contents", got)
	}
}

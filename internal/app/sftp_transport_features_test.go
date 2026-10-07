//go:build linux || darwin

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sshcSFTP "sshc/internal/sftp"
)

// The fixture crosses multiple data packets to catch framing errors during transfer.
const transportFixtureRepetitions = 20_000

func TestPooledSSHSubsystemPreservesContentsAndSupportsRemoteMetadataOperations(t *testing.T) {
	server := startSFTPSubsystemServer(t)
	pool := sshcSFTP.NewRemotePool(func(context.Context, string) (sshcSFTP.RemoteTarget, error) {
		return sshcSFTP.RemoteTarget{Identity: "fixture", Open: func(context.Context) (sshcSFTP.Remote, error) {
			return server.open(t), nil
		}}, nil
	})
	t.Cleanup(func() { _ = pool.Close() })
	service := sshcSFTP.Service{Open: pool.Open}
	directory := t.TempDir()
	contents := strings.Repeat("SFTP contents\n", transportFixtureRepetitions)
	for _, name := range []string{"notes.txt", "other.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	remoteDirectory := remotePathOf(directory)
	remoteFile := remotePathOf(filepath.Join(directory, "notes.txt"))
	text, err := service.ReadText(t.Context(), "fixture", remoteFile)
	if err != nil || text.Contents != contents {
		t.Fatalf("file contents changed over the SSH subsystem: %v", err)
	}
	listed, err := service.ListDirectory(t.Context(), "fixture", remoteDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed.Entries {
		if entry.Ownership == nil {
			t.Fatalf("server-supplied owners missing from %s", entry.Name)
		}
		stat, err := service.Stat(t.Context(), "fixture", entry.Path)
		if err != nil || stat.Ownership == nil || *stat.Ownership != *entry.Ownership || stat.Revision != entry.Revision {
			t.Fatalf("list and stat disagree for %s: %v", entry.Name, err)
		}
		_, err = service.ChangeOwnership(t.Context(), "fixture", sshcSFTP.OwnershipChange{
			Path: entry.Path, UID: entry.Ownership.UID, GID: entry.Ownership.GID, ExpectedRevision: entry.Revision,
		})
		if err != nil {
			t.Fatalf("ownership operation lost through the pool: %v", err)
		}
	}
	linkPath := remotePathOf(filepath.Join(directory, "current"))
	link, err := service.CreateSymlink(t.Context(), "fixture", sshcSFTP.SymlinkChange{Path: linkPath, Target: "notes.txt"})
	if err != nil || link.LinkTarget != "notes.txt" {
		t.Fatalf("create link through the pool: %v", err)
	}
	changed, err := service.ChangeSymlink(t.Context(), "fixture", sshcSFTP.SymlinkChange{
		Path: linkPath, Target: "other.txt", ExpectedRevision: link.Revision,
	})
	if err != nil || changed.LinkTarget != "other.txt" || changed.Revision == link.Revision {
		t.Fatalf("change link through the pool: %v", err)
	}
	space, err := service.FilesystemSpace(t.Context(), "fixture", remoteDirectory)
	if err != nil || space.TotalBytes == 0 || space.AvailableBytes > space.TotalBytes {
		t.Fatalf("filesystem space through the pool: %+v, %v", space, err)
	}
	for _, name := range []string{"notes.txt", "other.txt"} {
		actual, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || string(actual) != contents {
			t.Fatalf("metadata operation changed %s: %v", name, err)
		}
	}
	updatedContents := contents + "saved\n"
	if _, err := service.SaveText(t.Context(), "fixture", remoteFile, updatedContents, text.Revision); err != nil {
		t.Fatalf("save through the SSH subsystem: %v", err)
	}
	actual, err := os.ReadFile(filepath.Join(directory, "notes.txt"))
	if err != nil || string(actual) != updatedContents {
		t.Fatalf("saved file contents changed over the SSH subsystem: %v", err)
	}
}

package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestLocalAndRemoteComparisonReportsNestedDifferencesWithoutWriting(t *testing.T) {
	folder := t.TempDir()
	if err := os.Mkdir(filepath.Join(folder, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(folder, "nested", "same.txt")
	if err := os.WriteFile(localFile, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(localFile, testTime, testTime); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "local.txt"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	localInfo, err := os.Stat(localFile)
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteWith(map[string]node{
		"/work":                 directory("work"),
		"/work/nested":          directory("nested"),
		"/work/nested/same.txt": {name: "same.txt", mode: localInfo.Mode(), content: []byte("same"), modTime: testTime},
		"/work/remote.txt":      file("remote.txt", "remote", 0o600),
	})
	openedAliases := []string{}
	service := sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		openedAliases = append(openedAliases, alias)
		return remote, nil
	}}
	comparison, err := service.CompareDirectories(context.Background(), "sshc://local", filepath.ToSlash(folder), "edge", "/work")
	if err != nil {
		t.Fatal(err)
	}
	statuses := comparisonStatuses(comparison)
	if statuses["nested/same.txt"] != sftp.DirectorySame || statuses["local.txt"] != sftp.DirectoryLeftOnly || statuses["remote.txt"] != sftp.DirectoryRightOnly {
		t.Fatalf("comparison = %#v", statuses)
	}
	if len(openedAliases) != 1 || openedAliases[0] != "edge" {
		t.Fatalf("opened aliases = %v", openedAliases)
	}
	contents, err := os.ReadFile(localFile)
	if err != nil || string(contents) != "same" {
		t.Fatalf("local contents after comparison = %q, %v", contents, err)
	}
}

func TestLocalComparisonListsLinksWithoutFollowingTheirTargets(t *testing.T) {
	left, right, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(left, "link")); err != nil {
		t.Skipf("symlink fixture is unavailable: %v", err)
	}
	service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
		t.Fatal("local comparison opened an SSH connection")
		return nil, fs.ErrInvalid
	}}
	comparison, err := service.CompareDirectories(context.Background(), "sshc://local", filepath.ToSlash(left), "sshc://local", filepath.ToSlash(right))
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Entries) != 1 || comparison.Entries[0].RelativePath != "link" || comparison.Entries[0].Left.Type != sftp.EntrySymlink {
		t.Fatalf("comparison followed a link: %+v", comparison.Entries)
	}
	if _, err := os.Stat(filepath.Join(outside, "private.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestLocalComparisonRejectsRelativePathsAndHonorsCancellation(t *testing.T) {
	service := sftp.Service{}
	folder := filepath.ToSlash(t.TempDir())
	if _, err := service.CompareDirectories(context.Background(), "sshc://local", "relative", "sshc://local", folder); !errors.Is(err, sftp.ErrInvalidPath) {
		t.Fatalf("relative comparison = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.CompareDirectories(ctx, "sshc://local", folder, "sshc://local", folder); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled comparison = %v", err)
	}
}

func TestLocalComparisonReportsSizeAndModificationTimeChanges(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	for _, fixture := range []struct {
		folder, contents string
		modified         time.Time
	}{{left, "short", testTime}, {right, "longer", testTime.Add(time.Second)}} {
		filename := filepath.Join(fixture.folder, "changed.txt")
		if err := os.WriteFile(filename, []byte(fixture.contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filename, fixture.modified, fixture.modified); err != nil {
			t.Fatal(err)
		}
	}
	comparison, err := (sftp.Service{}).CompareDirectories(context.Background(), "sshc://local", filepath.ToSlash(left), "sshc://local", filepath.ToSlash(right))
	if err != nil {
		t.Fatal(err)
	}
	if comparisonStatuses(comparison)["changed.txt"] != sftp.DirectoryDifferent {
		t.Fatalf("comparison = %+v", comparison.Entries)
	}
}

func comparisonStatuses(comparison sftp.DirectoryComparison) map[string]sftp.DirectoryDifferenceStatus {
	statuses := make(map[string]sftp.DirectoryDifferenceStatus)
	for _, difference := range comparison.Entries {
		statuses[difference.RelativePath] = difference.Status
	}
	return statuses
}

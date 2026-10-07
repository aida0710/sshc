package sftp_test

import (
	"errors"
	"io/fs"
	"strconv"
	"testing"

	"sshc/internal/sftp"
)

// 検索・容量の集計・再帰 chmod は同じ走査の予算を使う。予算を超えた木では、検索と
// 集計は途中までの結果に Truncated を付け、chmod は何も変えずに断る。

func deepTree(root string, depth int) map[string]node {
	nodes := map[string]node{root: directory("srv")}
	deepest := root
	for range depth {
		deepest += "/d"
		nodes[deepest] = directory("d")
	}
	return nodes
}

func TestBoundedWalkReachesATreeExactlyAsDeepAsItsBudget(t *testing.T) {
	remote := remoteWith(deepTree("/srv", sftp.MaxSearchDepthForTest))
	service := serviceFor(remote)

	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/srv", Query: "d"})
	if err != nil || found.Truncated || len(found.Entries) != sftp.MaxSearchDepthForTest {
		t.Fatalf("Search() = %d entries, truncated %v, %v; want all %d", len(found.Entries), found.Truncated, err, sftp.MaxSearchDepthForTest)
	}
	stats, err := service.DirectoryStats(t.Context(), "edge", "/srv")
	if err != nil || stats.Truncated || stats.Directories != sftp.MaxSearchDepthForTest+1 {
		t.Fatalf("DirectoryStats() = %+v, %v", stats, err)
	}
}

func TestBoundedWalkStopsAtATreeDeeperThanItsBudget(t *testing.T) {
	remote := remoteWith(deepTree("/srv", sftp.MaxSearchDepthForTest+1))
	service := serviceFor(remote)

	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/srv", Query: "d"})
	if err != nil || !found.Truncated {
		t.Fatalf("Search() = truncated %v, %v; want truncated", found.Truncated, err)
	}
	stats, err := service.DirectoryStats(t.Context(), "edge", "/srv")
	if err != nil || !stats.Truncated {
		t.Fatalf("DirectoryStats() = %+v, %v; want truncated", stats, err)
	}
	assertChmodRecursiveRefusedWithoutChanges(t, remote, sftp.ErrTraversalLimit)
}

func TestBoundedWalkStopsAtMoreEntriesThanItsBudget(t *testing.T) {
	nodes := map[string]node{"/srv": directory("srv")}
	for index := range sftp.MaxSearchVisitedForTest + 1 {
		name := "f" + strconv.Itoa(index)
		nodes["/srv/"+name] = file(name, "", 0o644)
	}
	remote := remoteWith(nodes)
	service := serviceFor(remote)

	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/srv", Query: "absent"})
	if err != nil || !found.Truncated {
		t.Fatalf("Search() = truncated %v, %v; want truncated", found.Truncated, err)
	}
	stats, err := service.DirectoryStats(t.Context(), "edge", "/srv")
	if err != nil || !stats.Truncated || stats.Files != sftp.MaxSearchVisitedForTest {
		t.Fatalf("DirectoryStats() = %+v, %v; want %d files and truncated", stats, err, sftp.MaxSearchVisitedForTest)
	}
	assertChmodRecursiveRefusedWithoutChanges(t, remote, sftp.ErrTraversalLimit)
}

func TestBoundedWalkSkipsAnUnreadableBranchOnlyForSearchAndStats(t *testing.T) {
	remote := remoteWith(map[string]node{
		"/srv":               directory("srv"),
		"/srv/locked":        directory("locked"),
		"/srv/locked/a.log":  file("a.log", "hidden", 0o644),
		"/srv/open":          directory("open"),
		"/srv/open/b.log":    file("b.log", "visible", 0o644),
		"/srv/open/deeper":   directory("deeper"),
		"/srv/open/deeper/c": file("c", "visible", 0o644),
	})
	remote.readDirHook = func(directory string) error {
		if directory == "/srv/locked" {
			return fs.ErrPermission
		}
		return nil
	}
	service := serviceFor(remote)

	found, err := service.Search(t.Context(), sftp.SearchOptions{Alias: "edge", Path: "/srv", Query: ".log"})
	if err != nil || !found.Truncated || len(found.Entries) != 1 || found.Entries[0].Path != "/srv/open/b.log" {
		t.Fatalf("Search() = %+v, %v; want only /srv/open/b.log and truncated", found, err)
	}
	stats, err := service.DirectoryStats(t.Context(), "edge", "/srv")
	if err != nil || !stats.Truncated || stats.Files != 2 {
		t.Fatalf("DirectoryStats() = %+v, %v; want the 2 readable files and truncated", stats, err)
	}
	assertChmodRecursiveRefusedWithoutChanges(t, remote, fs.ErrPermission)
}

func assertChmodRecursiveRefusedWithoutChanges(t *testing.T, remote *fakeRemote, want error) {
	t.Helper()
	service := serviceFor(remote)
	root, err := service.Stat(t.Context(), "edge", "/srv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChmodRecursive(t.Context(), "edge", "/srv", 0o700, root.Revision); !errors.Is(err, want) {
		t.Fatalf("ChmodRecursive() = %v, want %v", err, want)
	}
	for candidate, entry := range remote.nodes {
		if entry.Mode().Perm() == 0o700 {
			t.Fatalf("ChmodRecursive changed %s although it refused the tree", candidate)
		}
	}
}

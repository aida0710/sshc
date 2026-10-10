package sftp_test

import (
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestContentSearchKeepsResultsWhenAnAncestorGetsAnUnrelatedSibling(t *testing.T) {
	remote := contentRemote(map[string]node{
		"/work": directory("work"), "/work/notes.txt": contentFixtureFile("notes.txt", "needle"),
	})
	remote.onRead = func(string) {
		ancestor := remote.nodes["/"]
		ancestor.modTime = ancestor.modTime.Add(time.Second)
		remote.nodes["/"] = ancestor
	}
	found, err := contentService(remote).Search(t.Context(), sftp.SearchOptions{Alias: "fixture", Path: "/work", Query: "needle", Mode: sftp.SearchContent})
	if err != nil || len(found.Matches) != 1 || len(found.Omissions) != 0 {
		t.Fatalf("unrelated ancestor update changed the search: %+v, %v", found, err)
	}
}

func TestChmodKeepsConfirmationWhenAnAncestorGetsAnUnrelatedSibling(t *testing.T) {
	remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{"/work": directory("work"), "/work/notes.txt": file("notes.txt", "contents", 0o600)})}
	service := permissionService(remote)
	request := sftp.ChmodRequest{Alias: "fixture", Entries: chmodSelection(t, service, []string{"/work/notes.txt"}),
		Options: sftp.ChmodOptions{FileMode: 0o640, DirectoryMode: 0o750}}
	plan, err := service.PrepareChmod(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	ancestor := remote.nodes["/"]
	ancestor.modTime = ancestor.modTime.Add(time.Second)
	remote.nodes["/"] = ancestor
	changed, err := plan.Apply(t.Context(), plan.Revision)
	if err != nil || changed.Applied != 1 || remote.nodes["/work/notes.txt"].mode.Perm() != 0o640 {
		t.Fatalf("unrelated ancestor update invalidated confirmation: %+v, %v", changed, err)
	}
}

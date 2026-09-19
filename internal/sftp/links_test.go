package sftp_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"sshc/internal/sftp"
)

func symlink(name, target string) node {
	return node{name: name, content: []byte(target), mode: fs.ModeSymlink | 0o777, modTime: testTime}
}

func linkedTree() map[string]node {
	return map[string]node{
		"/srv":            directory("srv"),
		"/srv/data":       directory("data"),
		"/srv/notes.txt":  file("notes.txt", "hello", 0o640),
		"/srv/data-link":  symlink("data-link", "data"),
		"/srv/notes-link": symlink("notes-link", "/srv/notes.txt"),
		"/srv/twice":      symlink("twice", "notes-link"),
		"/srv/broken":     symlink("broken", "missing"),
		"/srv/loop-a":     symlink("loop-a", "loop-b"),
		"/srv/loop-b":     symlink("loop-b", "loop-a"),
	}
}

func TestListingSaysWhereEachSymlinkPointsAndWhatItIs(t *testing.T) {
	listing, err := serviceFor(remoteWith(linkedTree())).ListDirectory(context.Background(), "edge", "/srv")
	if err != nil {
		t.Fatalf("ListDirectory() = %v", err)
	}
	byName := make(map[string]sftp.Entry)
	for _, entry := range listing.Entries {
		byName[entry.Name] = entry
	}
	// A link to a directory lists among the directories.
	if listing.Entries[0].Name != "data" || listing.Entries[1].Name != "data-link" {
		t.Errorf("listing order = %q %q, want data then data-link first", listing.Entries[0].Name, listing.Entries[1].Name)
	}
	cases := []struct {
		name       string
		linkTarget string
		targetType sftp.LinkTargetType
		size       int64
	}{
		{name: "data-link", linkTarget: "data", targetType: sftp.LinkTargetDirectory},
		{name: "notes-link", linkTarget: "/srv/notes.txt", targetType: sftp.LinkTargetFile, size: 5},
		{name: "twice", linkTarget: "notes-link", targetType: sftp.LinkTargetFile, size: 5},
		{name: "broken", linkTarget: "missing", targetType: ""},
		{name: "loop-a", linkTarget: "loop-b", targetType: ""},
	}
	for _, want := range cases {
		got := byName[want.name]
		if got.Type != sftp.EntrySymlink || got.LinkTarget != want.linkTarget || got.TargetType != want.targetType {
			t.Errorf("%s = type %q link %q target %q, want symlink to %q of type %q", want.name, got.Type, got.LinkTarget, got.TargetType, want.linkTarget, want.targetType)
		}
		if want.size != 0 && (got.Size != want.size || !got.ModifiedAt.Equal(byName["notes.txt"].ModifiedAt)) {
			t.Errorf("%s size, modified = %d, %s; want the file's %d, %s", want.name, got.Size, got.ModifiedAt, want.size, byName["notes.txt"].ModifiedAt)
		}
	}
	if got := byName["notes.txt"]; got.LinkTarget != "" || got.TargetType != "" {
		t.Errorf("a regular file carries link details: %#v", got)
	}
}

func TestReadingThroughASymlinkKeepsTheLinksPathAndSavingRewritesTheTarget(t *testing.T) {
	remote := remoteWith(linkedTree())
	service := serviceFor(remote)
	opened, err := service.ReadText(context.Background(), "edge", "/srv/twice")
	if err != nil {
		t.Fatalf("ReadText(link) = %v", err)
	}
	if opened.Contents != "hello" || opened.Entry.Path != "/srv/twice" || opened.Entry.Name != "twice" {
		t.Fatalf("ReadText(link) = %q at %s", opened.Contents, opened.Entry.Path)
	}
	saved, err := service.SaveText(context.Background(), "edge", "/srv/twice", "changed", opened.Revision)
	if err != nil {
		t.Fatalf("SaveText(link) = %v", err)
	}
	if saved.Contents != "changed" || saved.Entry.Path != "/srv/twice" {
		t.Fatalf("SaveText(link) = %q at %s", saved.Contents, saved.Entry.Path)
	}
	if got := string(remote.nodes["/srv/notes.txt"].content); got != "changed" {
		t.Fatalf("target after save = %q, want %q", got, "changed")
	}
	for _, link := range []string{"/srv/twice", "/srv/notes-link"} {
		if remote.nodes[link].mode&fs.ModeSymlink == 0 {
			t.Fatalf("%s was replaced by a regular file", link)
		}
	}
}

func TestDownloadingASymlinkSendsTheFileItPointsTo(t *testing.T) {
	prepared, err := serviceFor(remoteWith(linkedTree())).PrepareDownload(context.Background(), "edge", "/srv/notes-link")
	if err != nil {
		t.Fatalf("PrepareDownload(link) = %v", err)
	}
	defer prepared.Close()
	var downloaded bytes.Buffer
	if _, err := prepared.WriteFrom(context.Background(), 0, &downloaded); err != nil {
		t.Fatalf("WriteFrom() = %v", err)
	}
	if downloaded.String() != "hello" || prepared.Size != 5 {
		t.Fatalf("download = %q (%d bytes)", downloaded.String(), prepared.Size)
	}
}

func TestOpeningABrokenOrLoopingSymlinkFails(t *testing.T) {
	service := serviceFor(remoteWith(linkedTree()))
	if _, err := service.ReadText(context.Background(), "edge", "/srv/broken"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadText(broken) = %v, want %v", err, fs.ErrNotExist)
	}
	if _, err := service.ReadText(context.Background(), "edge", "/srv/loop-a"); !errors.Is(err, sftp.ErrLinkLoop) {
		t.Fatalf("ReadText(loop) = %v, want %v", err, sftp.ErrLinkLoop)
	}
	if _, err := service.PrepareDownload(context.Background(), "edge", "/srv/data-link"); !errors.Is(err, sftp.ErrNotRegularFile) {
		t.Fatalf("PrepareDownload(directory link) = %v, want %v", err, sftp.ErrNotRegularFile)
	}
}

package sftp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenSSHContentSearchReadsMultipleFilesAndSkipsLinks(t *testing.T) {
	client := openOpenSSHTestClient(t)
	directory := newOpenSSHFixtureDirectory(t)
	for name, contents := range map[string]string{"first.txt": "header\nneedle here\n", "second.txt": "needle again\n"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(directory, "first.txt"), filepath.Join(directory, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	service := Service{Open: func(_ context.Context, _ string) (Remote, error) { return client, nil }}
	search, err := service.Search(t.Context(), SearchOptions{Alias: "fixture", Path: filepath.ToSlash(directory), Query: "needle", Mode: SearchContent})
	if err != nil {
		t.Fatal(err)
	}
	matchedLines := make(map[string]int)
	for _, match := range search.Matches {
		matchedLines[match.Entry.Name] = match.Line
	}
	if len(search.Matches) != 2 || matchedLines["first.txt"] != 2 || matchedLines["second.txt"] != 1 {
		t.Fatalf("search results = %+v", search)
	}
	if len(search.Omissions) != 1 || search.Omissions[0].Reason != "symlink" || search.Omissions[0].Count != 1 {
		t.Fatalf("link omission = %+v", search.Omissions)
	}
}

func TestOpenSSHHashesDetectDifferentContentsDespiteMatchingMetadata(t *testing.T) {
	client := openOpenSSHTestClient(t)
	directory := newOpenSSHFixtureDirectory(t)
	modified := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for name, contents := range map[string]string{"left": "left value", "right": "right text"} {
		root := filepath.Join(directory, name)
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		filePath := filepath.Join(root, "same-size.txt")
		if err := os.WriteFile(filePath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filePath, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	service := Service{Open: func(_ context.Context, _ string) (Remote, error) { return client, nil }}
	comparison, err := service.CompareDirectories(t.Context(), CompareOptions{
		Left:  ComparisonLocation{Alias: "left", Path: filepath.ToSlash(filepath.Join(directory, "left"))},
		Right: ComparisonLocation{Alias: "right", Path: filepath.ToSlash(filepath.Join(directory, "right"))},
		Mode:  ComparisonContent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Entries) != 1 || comparison.Entries[0].Status != DirectoryDifferent || comparison.BytesRead != 20 {
		t.Fatalf("content comparison = %+v", comparison)
	}
}

package sftp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalTextSaveKeepsModeAndRefusesStaleContentsOrReplacement(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "first\r\n")
	if err := os.Chmod(filename, 0o640); err != nil {
		t.Fatal(err)
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil || opened.Contents != "first\r\n" {
		t.Fatalf("open = %+v, %v", opened, err)
	}
	saved, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "二行目\n", ExpectedRevision: opened.Revision})
	if err != nil || saved.Contents != "二行目\n" || saved.Revision == opened.Revision {
		t.Fatalf("save = %+v, %v", saved, err)
	}
	if runtime.GOOS != "windows" && saved.Entry.Mode.Perm() != 0o640 {
		t.Fatalf("mode = %v", saved.Entry.Mode)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "stale", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale = %v", err)
	}
	// An external atomic replacement with the same contents and metadata is
	// still another file; a content hash alone would silently overwrite it.
	metadata, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement.txt")
	if err := os.WriteFile(replacement, []byte(saved.Contents), metadata.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, metadata.ModTime(), metadata.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, filename); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: saved.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("replacement = %v", err)
	}
	assertLocalMutationFile(t, filename, saved.Contents)
	children, err := os.ReadDir(directory)
	if err != nil || len(children) != 1 {
		t.Fatalf("staged files left = %v, %v", children, err)
	}
}

func TestLocalTextRefusesLinksBinaryOversizedFilesAndChangedComparisonEntries(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "text")
	link := filepath.Join(directory, "link.txt")
	symlinkLocalMutationFixture(t, filename, link)
	if _, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: link}); !errors.Is(err, ErrConflict) {
		t.Fatalf("link = %v", err)
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename, ExpectedRevision: "old metadata"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("pinned read = %v", err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: link, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("link save = %v", err)
	}
	for _, fixture := range []struct {
		contents []byte
		want     error
	}{{[]byte{'a', 0}, ErrNotUTF8}, {bytes.Repeat([]byte{'x'}, MaxEditableFileBytes+1), ErrTextTooLarge}} {
		if err := os.WriteFile(filename, fixture.contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename}); !errors.Is(err, fixture.want) {
			t.Fatalf("read = %v, want %v", err, fixture.want)
		}
	}
}

func TestLocalTextSaveProtectsTransferPathsAndCancelledRequests(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	request := LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := manager.SaveLocalText(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	manager.jobs["local_transfer"] = &transferJobRecord{job: TransferJob{Operation: RemotePut, SourcePath: filename, Status: TransferQueued}}
	if _, err := manager.SaveLocalText(t.Context(), request); !errors.Is(err, ErrConflict) {
		t.Fatalf("transfer = %v", err)
	}
	assertLocalMutationFile(t, filename, "before")
}

func TestLocalPreviewRecognizesRasterBytesAndRefusesActiveContentAndLinks(t *testing.T) {
	_, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "image.txt")
	image := []byte("\x89PNG\r\n\x1a\npreview")
	if err := os.WriteFile(filename, image, 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := ReadLocalPreview(t.Context(), filename)
	if err != nil || preview.ContentType != "image/png" || !bytes.Equal(preview.Contents, image) {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	writeLocalMutationFile(t, filename, `<svg onload="alert(1)"></svg>`)
	if _, err := ReadLocalPreview(t.Context(), filename); !errors.Is(err, ErrPreviewType) {
		t.Fatalf("active content = %v", err)
	}
	link := filepath.Join(directory, "link")
	symlinkLocalMutationFixture(t, filename, link)
	if _, err := ReadLocalPreview(t.Context(), link); !errors.Is(err, ErrConflict) {
		t.Fatalf("link = %v", err)
	}
}

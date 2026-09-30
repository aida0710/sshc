package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 中身を持たずに求めた digest は、読んでから求めた Digest と同じで、ReadFileLimited と
// 同じものを断る。
func TestDigestFileLimitedMatchesDigestAndRefusesWhatReadingRefuses(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "background.png")
	contents := bytes.Repeat([]byte("background "), 4096)
	if err := os.WriteFile(target, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	limit := int64(len(contents))

	got, err := DigestFileLimited(OSFileSystem{}, target, limit)
	if err != nil || got != Digest(contents) {
		t.Fatalf("DigestFileLimited = %q, %v; want %q", got, err, Digest(contents))
	}
	if _, err := DigestFileLimited(OSFileSystem{}, target, limit-1); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("DigestFileLimited over the limit = %v, want ErrFileTooLarge", err)
	}
	if _, err := DigestFileLimited(OSFileSystem{}, directory, limit); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("DigestFileLimited(directory) = %v, want ErrNotRegularFile", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(directory, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := DigestFileLimited(OSFileSystem{}, link, limit); err == nil {
			t.Fatal("DigestFileLimited followed a symbolic link")
		}
	}
}

// 包んだ FileSystem が digest の近道を持たなければ、ReadFileLimited で読んでから求める。
// 障害注入の fake は、これまでどおり ReadFile で失敗を差し込める。
func TestDigestFileLimitedFallsBackToReadingThroughAWrappedFileSystem(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "config")
	if err := os.WriteFile(target, []byte("Host example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected read failure")
	wrapped := failingReadFileSystem{FileSystem: OSFileSystem{}, failure: injected}
	if _, err := DigestFileLimited(wrapped, target, MaxFileSize); !errors.Is(err, injected) {
		t.Fatalf("DigestFileLimited through a wrapper = %v, want the wrapper's ReadFile failure", err)
	}
}

// ワークスペースの非公開状態は、digest を求めるときも、読み取りと同じく最終ハンドルを
// 検証する読み口を通る。
func TestWorkspaceDigestsPrivateStateThroughTheAuthenticatedReader(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &privateReadTrackingFileSystem{FileSystem: OSFileSystem{}}
	workspace, err := NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(workspace.StateDir(), "backgrounds", "large.png")
	managedSSHPath := filepath.Join(workspace.Root(), "config")

	privateDigest, err := DigestFileLimited(workspace.FileSystem(), statePath, MaxFileSize)
	if err != nil || privateDigest != Digest([]byte("private")) {
		t.Fatalf("private digest = %q, %v", privateDigest, err)
	}
	regularDigest, err := DigestFileLimited(workspace.FileSystem(), managedSSHPath, MaxFileSize)
	if err != nil || regularDigest != Digest([]byte("regular")) {
		t.Fatalf("regular digest = %q, %v", regularDigest, err)
	}
	if got, want := fileSystem.privateReads, []string{statePath}; !samePaths(got, want) {
		t.Fatalf("private reads = %#v, want %#v", got, want)
	}
}

type failingReadFileSystem struct {
	FileSystem
	failure error
}

func (f failingReadFileSystem) ReadFile(string) ([]byte, error) { return nil, f.failure }

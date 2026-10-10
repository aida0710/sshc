package app

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/application"
	"sshc/internal/platform/windowsacl/acltest"
)

// leftoverBackgroundUploadNameは、背景画像を受け取っている途中に落ちて残った一時ファイルの
// 名前である。先頭はapplicationの一時ファイルの先頭（".sshc-background-"）に合わせる。
const leftoverBackgroundUploadName = ".sshc-background-0123abcd"

// sshcエンジンを組むときに、背景画像を受け取っている途中に落ちて残った一時ファイルを消す。
// 置いてある画像には触れない。
func TestBuildingTheEngineRemovesLeftoverBackgroundUploads(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, ".ssh", filepath.FromSlash(application.BackgroundsDirectory))
	leftover := filepath.Join(directory, leftoverBackgroundUploadName)
	stored := filepath.Join(directory, "office.png")
	acltest.WritePrivateFile(t, leftover, []byte("\x89PNG\r\n\x1a\npartial"))
	acltest.WritePrivateFile(t, stored, []byte("\x89PNG\r\n\x1a\nkept"))

	if _, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("leftover upload: %v, want it removed", err)
	}
	if _, err := os.Lstat(stored); err != nil {
		t.Errorf("stored image was not kept: %v", err)
	}
}

//go:build windows

package enginelog

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
	"sshc/internal/platform/windowsacl"
)

func TestLogAndDirectoryHavePrivateACLs(t *testing.T) {
	stateDir := t.TempDir()
	file, err := Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	for _, path := range []string{Path(stateDir), filepath.Dir(Path(stateDir))} {
		private, err := windowsacl.IsRestrictedToCurrentUser(path)
		if err != nil || !private {
			t.Fatalf("ACL for %s: private = %v, err = %v", path, private, err)
		}
	}
}

// メモ帳や type、Get-Content は、削除の共有を許さずに読む。エンジンが追記している
// あいだも、その読み方でログを開けなければならない。
func TestOtherProgramsCanReadTheLogWhileTheEngineAppends(t *testing.T) {
	stateDir := t.TempDir()
	log, err := Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if _, err := log.WriteString("still running\n"); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(Path(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("a reader that does not share deletion cannot open the log while it is appended to: %v", err)
	}
	windows.CloseHandle(reader)
}

//go:build windows

package storage

import (
	"io"
	"path/filepath"
	"testing"
)

// 保存するあいだにも、画面の読み直しや Windows Defender の検査が metadata.json などを開いている。
// 開いている人がいても、置き換えが Access is denied で断られないことを、公開の3つの道で確かめる。
func TestPublishingReplacesAFileThatIsOpenForReading(t *testing.T) {
	fileSystem := OSFileSystem{}
	publishers := map[string]func(t *testing.T, target string, contents []byte) error{
		"WriteAtomic": func(t *testing.T, target string, contents []byte) error {
			return fileSystem.WriteAtomic(target, ".sshc-test-", FilePermission, contents)
		},
		"Rename": func(t *testing.T, target string, contents []byte) error {
			return fileSystem.Rename(writeTestTemporary(t, target, contents), target)
		},
		"MovePrivate": func(t *testing.T, target string, contents []byte) error {
			return fileSystem.MovePrivate(writeTestTemporary(t, target, contents), target)
		},
	}
	for name, publish := range publishers {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "metadata.json")
			if err := fileSystem.WriteAtomic(target, ".sshc-test-", FilePermission, []byte("old")); err != nil {
				t.Fatal(err)
			}
			reader, err := openRegularNoFollow(target)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			if err := publish(t, target, []byte("new")); err != nil {
				t.Fatalf("replace a file that is open for reading: %v", err)
			}
			if contents, err := fileSystem.ReadPrivateFile(target); err != nil || string(contents) != "new" {
				t.Fatalf("published contents = %q, %v; want new", contents, err)
			}
			// 開いていた人は、置き換える前の中身を最後まで読める。
			if held, err := io.ReadAll(reader); err != nil || string(held) != "old" {
				t.Fatalf("contents through the earlier handle = %q, %v; want old", held, err)
			}
		})
	}
}

func writeTestTemporary(t *testing.T, target string, contents []byte) string {
	t.Helper()
	temporary, err := (OSFileSystem{}).WriteTemp(filepath.Dir(target), ".sshc-test-", FilePermission, contents)
	if err != nil {
		t.Fatal(err)
	}
	return temporary
}

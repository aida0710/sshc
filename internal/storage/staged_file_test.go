package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/iotest"
)

// stagingTestPrefix は、テストで作る一時ファイルの名前の先頭である。
const stagingTestPrefix = ".sshc-staging-test-"

// patternReader は、決まった並びのバイトを remaining バイトまで返す。大きな中身を
// メモリに置かずに作り、consumed で読まれた量を数える。
type patternReader struct {
	remaining int64
	consumed  int64
}

func (r *patternReader) Read(buffer []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	count := int(min(int64(len(buffer)), r.remaining))
	for index := range count {
		buffer[index] = byte((r.consumed + int64(index)) % 251)
	}
	r.remaining -= int64(count)
	r.consumed += int64(count)
	return count, nil
}

func directoryNames(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestAPublishedStagedFileHoldsTheWholeSourceUnderTheChosenName(t *testing.T) {
	directory := t.TempDir()
	// io.Copy の 1 回の読み取り（32 KiB）より大きくし、何回にも分けて書かせる。
	const size = 1<<20 + 7
	staged, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     &patternReader{remaining: size},
		Maximum:    size,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if staged.Size() != size {
		t.Fatalf("size = %d, want %d", staged.Size(), size)
	}
	target := filepath.Join(directory, "image.png")
	if err := staged.Publish(target); err != nil {
		t.Fatal(err)
	}
	staged.Discard()

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	want, err := io.ReadAll(&patternReader{remaining: size})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, want) {
		t.Fatal("the published file does not hold the bytes that were read")
	}
	if names := directoryNames(t, directory); len(names) != 1 || names[0] != "image.png" {
		t.Fatalf("directory = %q, want only the published file", names)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != FilePermission {
			t.Fatalf("permission = %04o, want %04o", info.Mode().Perm(), FilePermission)
		}
	}
}

func TestStagingSourceBeyondTheMaximumIsRefusedAndLeavesNothing(t *testing.T) {
	directory := t.TempDir()
	const maximum = 64 << 10
	source := &patternReader{remaining: 1 << 40}
	_, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     source,
		Maximum:    maximum,
	})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if source.consumed > maximum+1 {
		t.Fatalf("read %d bytes, want reading to stop one byte past %d", source.consumed, maximum)
	}
	if names := directoryNames(t, directory); len(names) != 0 {
		t.Fatalf("directory = %q, want the temporary file removed", names)
	}
}

func TestStagingSourceExactlyAtTheMaximumIsAccepted(t *testing.T) {
	directory := t.TempDir()
	const maximum = 64 << 10
	staged, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     &patternReader{remaining: maximum},
		Maximum:    maximum,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if staged.Size() != maximum {
		t.Fatalf("size = %d, want %d", staged.Size(), maximum)
	}
}

func TestASourceThatBreaksOffWhileStagingIsReportedAsUnreadableAndLeavesNothing(t *testing.T) {
	directory := t.TempDir()
	cause := errors.New("connection reset")
	_, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     io.MultiReader(&patternReader{remaining: 100 << 10}, iotest.ErrReader(cause)),
		Maximum:    1 << 20,
	})
	if !errors.Is(err, ErrSourceUnreadable) || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want ErrSourceUnreadable wrapping the read failure", err)
	}
	if names := directoryNames(t, directory); len(names) != 0 {
		t.Fatalf("directory = %q, want the temporary file removed", names)
	}
}

func TestDiscardingAStagedFileLeavesNothing(t *testing.T) {
	directory := t.TempDir()
	staged, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     &patternReader{remaining: 4 << 10},
		Maximum:    1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	names := directoryNames(t, directory)
	if len(names) != 1 || !IsTemporaryName(names[0]) {
		t.Fatalf("directory while staged = %q, want one temporary file", names)
	}
	staged.Discard()
	staged.Discard()
	if names := directoryNames(t, directory); len(names) != 0 {
		t.Fatalf("directory = %q, want the temporary file removed", names)
	}
}

func TestAStagedFileIsNotPublishedOutsideItsDirectory(t *testing.T) {
	directory := t.TempDir()
	elsewhere := t.TempDir()
	staged, err := OSFileSystem{}.StageFile(StageRequest{
		Directory:  directory,
		Prefix:     stagingTestPrefix,
		Permission: FilePermission,
		Source:     &patternReader{remaining: 16},
		Maximum:    1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Publish(filepath.Join(elsewhere, "image.png")); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("err = %v, want publishing into another directory refused", err)
	}
	staged.Discard()
	if names := directoryNames(t, elsewhere); len(names) != 0 {
		t.Fatalf("other directory = %q, want nothing placed there", names)
	}
	if names := directoryNames(t, directory); len(names) != 0 {
		t.Fatalf("directory = %q, want the temporary file removed", names)
	}
}

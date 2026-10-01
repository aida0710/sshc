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

// stagingTestPrefixは、テストで作る一時ファイルの名前の先頭である。
const stagingTestPrefix = ".sshc-staging-test-"

// patternPeriodは、patternReaderが繰り返す並びの長さである。256より小さい素数にして、
// 2の冪の読み取りの区切り（io.Copyの32 KiBなど）と周期を揃えない。区切りでずれたり
// 重なったりした塊を、中身の比較で見つけられる。
const patternPeriod = 251

// patternReaderは、決まった並びのバイトをremainingバイトまで返す。大きな中身をメモリに
// 置かずに作り、consumedで読まれた量を数える。application/background_upload_test.goにも
// 同じ形のものがある（テストの間で共有する場が無い）。
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
		buffer[index] = byte((r.consumed + int64(index)) % patternPeriod)
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
	// io.Copyの1回の読み取り（32 KiB）より大きくし、何回にも分けて書かせる。
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

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("left"), FilePermission); err != nil {
		t.Fatal(err)
	}
}

// プロセスが落ちて残った一時ファイルだけを消し、置き場所のほかのファイルや、変更の記録が
// 復旧に使う一時ファイルには触れない。
func TestRemovingLeftoverStagedFilesRemovesOnlyThatPrefix(t *testing.T) {
	directory := t.TempDir()
	leftover := filepath.Join(directory, stagingTestPrefix+"0123abcd")
	writeTestFile(t, leftover)
	kept := []string{"image.png", ".sshc-20261001T120000.000-0badc0de-config", ".sshc-other-0123abcd"}
	for _, name := range kept {
		writeTestFile(t, filepath.Join(directory, name))
	}
	if err := os.Mkdir(filepath.Join(directory, stagingTestPrefix+"directory"), DirectoryPermission); err != nil {
		t.Fatal(err)
	}

	if err := RemoveLeftoverStagedFiles(OSFileSystem{}, directory, stagingTestPrefix); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover staged file: %v, want it removed", err)
	}
	for _, name := range append(kept, stagingTestPrefix+"directory") {
		if _, err := os.Lstat(filepath.Join(directory, name)); err != nil {
			t.Errorf("%s was not kept: %v", name, err)
		}
	}
}

// ".sshc-"だけや、一時ファイルの印の無い先頭では、ほかの一時ファイルや利用者のファイルまで
// 消しうるので断る。
func TestRemovingLeftoverStagedFilesRefusesAPrefixNotOfItsOwn(t *testing.T) {
	directory := t.TempDir()
	names := []string{".sshc-20261001T120000.000-0badc0de-config", "background-0123abcd.png"}
	for _, name := range names {
		writeTestFile(t, filepath.Join(directory, name))
	}
	for _, prefix := range []string{"", ".sshc-", ".SSHC-", "background"} {
		if err := RemoveLeftoverStagedFiles(OSFileSystem{}, directory, prefix); !errors.Is(err, os.ErrInvalid) {
			t.Errorf("prefix %q: err = %v, want it refused", prefix, err)
		}
	}
	if got := directoryNames(t, directory); len(got) != len(names) {
		t.Fatalf("directory = %q, want every file kept", got)
	}
}

func TestRemovingLeftoverStagedFilesFromAMissingDirectoryIsNotAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := RemoveLeftoverStagedFiles(OSFileSystem{}, missing, stagingTestPrefix); err != nil {
		t.Fatalf("err = %v, want nothing to remove", err)
	}
}

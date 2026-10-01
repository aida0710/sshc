package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"

	"sshc/internal/storage"
)

// countingReader は、読まれたバイト数を数える。
type countingReader struct {
	reader   io.Reader
	consumed int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	r.consumed += int64(count)
	return count, err
}

// generatedBody は、header のあとに size バイトの決まった並びを続ける本文を、
// メモリに置かずに作る。
func generatedBody(header []byte, size int64) *countingReader {
	return &countingReader{reader: io.MultiReader(bytes.NewReader(header), io.LimitReader(&patternSource{}, size))}
}

// patternSource は、0 から 250 を繰り返すバイトを尽きることなく返す。並びは
// 読み取りの区切り方によらず、本文の中の位置で決まる。
type patternSource struct {
	offset int64
}

func (r *patternSource) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = byte((r.offset + int64(index)) % 251)
	}
	r.offset += int64(len(buffer))
	return len(buffer), nil
}

func backgroundDirectoryNames(t *testing.T, workspace *storage.Workspace) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestAnUploadedBackgroundIsStoredWithTheBytesThatWereSent(t *testing.T) {
	service, workspace := newTerminalService(t)
	// 1 回の読み取りより大きくし、何回にも分けて書かせる。
	const size = 3 << 20
	added, err := service.AddBackground("photo", generatedBody([]byte(pngSignature), size))
	if err != nil {
		t.Fatal(err)
	}
	want, err := io.ReadAll(generatedBody([]byte(pngSignature), size))
	if err != nil {
		t.Fatal(err)
	}
	if added.Name != "photo.png" || added.Bytes != len(want) || added.Type != "image/png" {
		t.Fatalf("added = %#v", added)
	}
	stored, err := os.ReadFile(filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory), added.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, want) {
		t.Fatal("the stored image does not hold the bytes that were sent")
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 1 {
		t.Fatalf("directory = %q, want only the stored image", names)
	}
}

func TestABackgroundLargerThanTheRoomLeftIsRefusedWithoutReadingItAll(t *testing.T) {
	service, workspace := newTerminalService(t)
	room := int64(DefaultBackgroundCapacityMiB) << 20
	body := generatedBody([]byte(pngSignature), 1<<40)

	if _, err := service.AddBackground("huge", body); !errors.Is(err, ErrBackgroundsFull) {
		t.Fatalf("err = %v, want ErrBackgroundsFull", err)
	}
	if body.consumed > room+1 {
		t.Fatalf("read %d bytes, want reading to stop one byte past the %d bytes left", body.consumed, room)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 0 {
		t.Fatalf("directory = %q, want neither a temporary file nor an image", names)
	}
}

func TestABackgroundWhoseBodyBreaksOffIsRefusedAndLeavesNothing(t *testing.T) {
	service, workspace := newTerminalService(t)
	cause := errors.New("connection reset")
	body := io.MultiReader(generatedBody([]byte(pngSignature), 256<<10), iotest.ErrReader(cause))

	_, err := service.AddBackground("photo", body)
	if !errors.Is(err, ErrBackgroundUnreadable) || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want ErrBackgroundUnreadable wrapping the read failure", err)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 0 {
		t.Fatalf("directory = %q, want neither a temporary file nor an image", names)
	}
}

func TestTheImageTypeIsDecidedFromTheLeadingBytesAlone(t *testing.T) {
	service, workspace := newTerminalService(t)
	body := generatedBody([]byte("<html>"), 1<<40)

	if _, err := service.AddBackground("page", body); !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want ErrNotAnImage", err)
	}
	if body.consumed > imageHeaderLength {
		t.Fatalf("read %d bytes, want only the first %d", body.consumed, imageHeaderLength)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 0 {
		t.Fatalf("directory = %q, want nothing written", names)
	}
}

func TestABackgroundWithoutAUsableNameIsNamedAfterItsContents(t *testing.T) {
	service, _ := newTerminalService(t)
	contents := png("anonymous")

	added, err := service.AddBackground("..", bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(contents)
	if want := "background-" + hex.EncodeToString(sum[:4]) + ".png"; added.Name != want {
		t.Fatalf("name = %q, want %q", added.Name, want)
	}
}

// 本文を受け取っている間は錠を持たない。そのあいだに同じ名前の画像が置かれても、
// 置く直前に一覧を読み直すので、先に置かれた画像を上書きしない。
func TestABackgroundAddedWhileAnotherIsUploadingDoesNotGetOverwritten(t *testing.T) {
	service, _ := newTerminalService(t)
	pipeReader, pipeWriter := io.Pipe()
	type outcome struct {
		background Background
		err        error
	}
	slow := make(chan outcome, 1)
	go func() {
		background, err := service.AddBackground("wall", pipeReader)
		slow <- outcome{background, err}
	}()
	if _, err := pipeWriter.Write(png("slow")); err != nil {
		t.Fatal(err)
	}

	fast, err := service.AddBackground("wall", bytes.NewReader(png("fast")))
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeWriter.Close(); err != nil {
		t.Fatal(err)
	}
	finished := <-slow
	if finished.err != nil {
		t.Fatal(finished.err)
	}
	if fast.Name != "wall.png" || finished.background.Name != "wall-2.png" {
		t.Fatalf("names = %q and %q, want the later one renamed", fast.Name, finished.background.Name)
	}
	contents, _, err := service.BackgroundContents(fast.Name)
	if err != nil || !bytes.Equal(contents, png("fast")) {
		t.Fatalf("%s = %q, %v; want the first image kept", fast.Name, contents, err)
	}
}

// 受け取っている途中の一時ファイルや、クラッシュで残った一時ファイルは、画像の
// 先頭を持っていても背景として一覧に出さない。
func TestATemporaryFileInTheBackgroundsDirectoryIsNotListed(t *testing.T) {
	service, workspace := newTerminalService(t)
	directory := filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory))
	if err := workspace.EnsureDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, backgroundTemporaryPrefix+"0123abcd"), png("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	listed, err := service.Backgrounds()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed = %#v, want the temporary file left out", listed)
	}
}

func TestOverflowingTheWholeAbsoluteMaximumIsTooLargeAndAnythingLessIsFull(t *testing.T) {
	if err := backgroundOverflow(MaxBackgroundBytes); !errors.Is(err, ErrBackgroundTooLarge) {
		t.Fatalf("overflow with the absolute maximum free = %v, want ErrBackgroundTooLarge", err)
	}
	if err := backgroundOverflow(MaxBackgroundBytes - 1); !errors.Is(err, ErrBackgroundsFull) {
		t.Fatalf("overflow with less free = %v, want ErrBackgroundsFull", err)
	}
	if err := backgroundOverflow(0); !errors.Is(err, ErrBackgroundsFull) {
		t.Fatalf("overflow with nothing free = %v, want ErrBackgroundsFull", err)
	}
}

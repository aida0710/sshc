package application

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

	"sshc/internal/storage"
)

// countingReaderは、読まれたバイト数を数える。
type countingReader struct {
	reader   io.Reader
	consumed int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	r.consumed += int64(count)
	return count, err
}

// generatedBodyは、headerのあとにsizeバイトの決まった並びを続ける本文を、メモリに置かずに
// 作る。読まれた量は、headerも含めて数える。
func generatedBody(header []byte, size int64) *countingReader {
	return &countingReader{reader: io.MultiReader(bytes.NewReader(header), &patternReader{remaining: size})}
}

// patternPeriodは、patternReaderが繰り返す並びの長さである。256より小さい素数にして、
// 2の冪の読み取りの区切り（io.Copyの32 KiBなど）と周期を揃えない。区切りでずれたり
// 重なったりした塊を、中身の比較で見つけられる。
const patternPeriod = 251

// patternReaderは、決まった並びのバイトをremainingバイトまで返す。大きな中身をメモリに
// 置かずに作り、consumedで読まれた量を数える。storage/staged_file_test.goにも同じ形の
// ものがある（テストの間で共有する場が無い）。
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
	// 1回の読み取りより大きくし、何回にも分けて書かせる。
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
	remaining := int64(DefaultBackgroundCapacityMiB) << 20
	body := generatedBody([]byte(pngSignature), 1<<40)

	if _, err := service.AddBackground("huge", body); !errors.Is(err, ErrBackgroundsFull) {
		t.Fatalf("err = %v, want ErrBackgroundsFull", err)
	}
	if body.consumed > remaining+1 {
		t.Fatalf("read %d bytes, want reading to stop one byte past the %d bytes left", body.consumed, remaining)
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

// 本文を受け取っている間は錠を持たない。そのあいだに同じ名前の画像が置かれても、置く直前に
// 一覧を読み直すので、先に置かれた画像を上書きしない。
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

// 受け取っている途中の一時ファイルや、クラッシュで残った一時ファイルは、画像の先頭を
// 持っていても背景として一覧に出さない。
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

// 長さを宣言した本文が先頭の途中で切れると、net/httpはio.ErrUnexpectedEOFを返し、その次から
// io.EOFを返す。短い本文と取り違えて、切れた画像を置いてはならない。
func TestABodyCutShortInsideTheLeadingBytesIsRefusedAsUnreadable(t *testing.T) {
	service, workspace := newTerminalService(t)
	// JPEGの印（3バイト）と1バイトだけを送り、宣言した長さの残りを送らずに切る。
	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"POST /api/v1/terminal/backgrounds HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Length: 1024\r\n\r\n\xff\xd8\xff\xe0")))
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.AddBackground("photo", request.Body)
	if !errors.Is(err, ErrBackgroundUnreadable) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want ErrBackgroundUnreadable wrapping io.ErrUnexpectedEOF", err)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 0 {
		t.Fatalf("directory = %q, want nothing stored", names)
	}
}

// 入口の上限（http.MaxBytesReader）で読み取りが止まったときも、その元のエラーを取り出せる。
// handlerは、それを見て送り方の誤りではなく大きすぎる画像として断る。本物の上限（1 GiB）を
// 超えるにはディスクへ1 GiB書くことになるので、同じ型のエラーを返す小さい上限で代える。
func TestABodyStoppedByTheRequestCeilingStillCarriesTheCeilingError(t *testing.T) {
	service, workspace := newTerminalService(t)
	const ceiling = 64 << 10
	body := http.MaxBytesReader(nil, io.NopCloser(generatedBody([]byte(pngSignature), 1<<40)), ceiling)

	_, err := service.AddBackground("photo", body)
	var overCeiling *http.MaxBytesError
	if !errors.As(err, &overCeiling) || !errors.Is(err, ErrBackgroundUnreadable) {
		t.Fatalf("err = %v, want ErrBackgroundUnreadable that still carries *http.MaxBytesError", err)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 0 {
		t.Fatalf("directory = %q, want neither a temporary file nor an image", names)
	}
}

// allocationAllowanceは、本文をメモリに載せないことを確かめるときに許す割り当ての量である。
// 本文（memoryProbeBodySize）の8分の1にして、一覧や設定の読み込み、io.Copyの32 KiBの緩衝が
// 混ざっても収まり、本文を丸ごと読めば必ず超えるようにする。
const (
	memoryProbeBodySize = 64 << 20
	allocationAllowance = memoryProbeBodySize / 8
)

// 本文はメモリに載せない。大きな本文を受け取っても、割り当てる量は本文よりずっと小さい。
func TestAddingABackgroundDoesNotHoldTheBodyInMemory(t *testing.T) {
	service, _ := newTerminalService(t)
	if _, err := service.SetBackgroundCapacityMiB(2 * memoryProbeBodySize >> 20); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	added, err := service.AddBackground("large", generatedBody([]byte(pngSignature), memoryProbeBodySize))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if added.Bytes != len(pngSignature)+memoryProbeBodySize {
		t.Fatalf("stored %d bytes, want the whole body", added.Bytes)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > allocationAllowance {
		t.Fatalf("allocated %d bytes for a %d byte body, want at most %d", allocated, memoryProbeBodySize, allocationAllowance)
	}
}

// sshcエンジンが受け取っている途中に落ちて残った一時ファイルは消す。置いてある画像には触れない。
func TestLeftoverUploadsAreRemovedAndStoredImagesKept(t *testing.T) {
	service, workspace := newTerminalService(t)
	stored, err := service.AddBackground("office", bytes.NewReader(png("kept")))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory))
	leftover := filepath.Join(directory, backgroundTemporaryPrefix+"0123abcd")
	if err := os.WriteFile(leftover, png("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := service.RemoveLeftoverBackgroundUploads(); err != nil {
		t.Fatal(err)
	}
	if names := backgroundDirectoryNames(t, workspace); len(names) != 1 || names[0] != stored.Name {
		t.Fatalf("directory = %q, want only %s", names, stored.Name)
	}
}

func TestRemovingLeftoverUploadsBeforeAnyBackgroundIsNotAnError(t *testing.T) {
	service, _ := newTerminalService(t)
	if err := service.RemoveLeftoverBackgroundUploads(); err != nil {
		t.Fatalf("err = %v, want nothing to remove", err)
	}
}

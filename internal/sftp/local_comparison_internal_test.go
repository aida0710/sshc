package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalComparisonStopsReadingOversizedDirectoriesIncludingInternalNames(t *testing.T) {
	// A million-entry listing must cost only the budget plus one overflow entry.
	const oversizedDirectoryEntries = 1_000_000
	reader := &generatedLocalComparisonDirectory{entryCount: oversizedDirectoryEntries}
	walker := localComparisonWalker{entries: make(map[string]Entry)}
	err := walker.walkDirectoryEntries(t.Context(), ".", reader)
	if !errors.Is(err, ErrCompareLimit) {
		t.Fatalf("oversized directory = %v, want ErrCompareLimit", err)
	}
	if reader.returnedEntries != maxComparedEntries+1 {
		t.Fatalf("read %d entries, want exactly %d", reader.returnedEntries, maxComparedEntries+1)
	}
	// Leave room to tune batching while requiring small, cancellable reads.
	const maximumListingBatch = 256
	if reader.largestBatch > maximumListingBatch {
		t.Fatalf("largest batch = %d, want at most %d", reader.largestBatch, maximumListingBatch)
	}
	if len(walker.entries) != 0 || len(walker.pendingDirectories) != 0 {
		t.Fatal("internal names were included or queued for traversal")
	}
}

func TestLocalComparisonCountsInternalNamesAcrossDirectoriesAt20000And20001(t *testing.T) {
	walker := localComparisonWalker{entries: make(map[string]Entry)}
	first := &generatedLocalComparisonDirectory{entryCount: maxComparedEntries - 1}
	if err := walker.walkDirectoryEntries(t.Context(), "first", first); err != nil {
		t.Fatal(err)
	}
	second := &generatedLocalComparisonDirectory{entryCount: 1, eofWithEntries: true}
	if err := walker.walkDirectoryEntries(t.Context(), "second", second); err != nil {
		t.Fatalf("20,000 internal names = %v", err)
	}
	if walker.scannedEntryCount != maxComparedEntries {
		t.Fatalf("scanned %d entries, want %d", walker.scannedEntryCount, maxComparedEntries)
	}
	third := &generatedLocalComparisonDirectory{entryCount: 1}
	if err := walker.walkDirectoryEntries(t.Context(), "third", third); !errors.Is(err, ErrCompareLimit) {
		t.Fatalf("20,001 internal names = %v, want ErrCompareLimit", err)
	}
	if third.lastRequest != 1 || third.returnedEntries != 1 {
		t.Fatalf("overflow listing = %+v, want exactly one entry", third)
	}
}

func TestLocalComparisonCancellationPreventsTheNextBatchRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := &generatedLocalComparisonDirectory{
		entryCount: maxComparedEntries,
		cancel:     cancel,
	}
	walker := localComparisonWalker{entries: make(map[string]Entry)}
	if err := walker.walkDirectoryEntries(ctx, ".", reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during the first batch = %v, want context.Canceled", err)
	}
	if reader.readCount != 1 {
		t.Fatalf("read %d batches after cancellation, want only the first", reader.readCount)
	}
}

func TestLocalComparisonCancellationPreventsAnyDirectoryRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reader := &generatedLocalComparisonDirectory{entryCount: 1}
	walker := localComparisonWalker{entries: make(map[string]Entry)}
	if err := walker.walkDirectoryEntries(ctx, ".", reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled listing = %v, want context.Canceled", err)
	}
	if reader.readCount != 0 {
		t.Fatalf("canceled listing performed %d reads", reader.readCount)
	}
}

func TestLocalComparisonKeepsTheOpenedRootWhenItsPathIsReplaced(t *testing.T) {
	folder := t.TempDir()
	selected := filepath.Join(folder, "selected")
	if err := os.Mkdir(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "original"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(selected)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(selected, filepath.Join(folder, "moved")); err != nil {
		t.Skipf("renaming an open directory is unavailable: %v", err)
	}
	if err := os.Mkdir(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "replacement"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	walker := localComparisonWalker{
		root: root, rootPath: selected,
		entries: make(map[string]Entry), pendingDirectories: []string{"."},
	}
	entries, err := walker.walk(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := entries["original"]; !exists || len(entries) != 1 {
		t.Fatalf("entries = %+v, want only the original directory's contents", entries)
	}
}

// Generate only the requested batch, so the test can model enormous listings
// without allocating those listings or depending on filesystem timing.
type generatedLocalComparisonDirectory struct {
	entryCount      int
	returnedEntries int
	readCount       int
	largestBatch    int
	lastRequest     int
	eofWithEntries  bool
	cancel          context.CancelFunc
}

func (directory *generatedLocalComparisonDirectory) ReadDir(count int) ([]os.DirEntry, error) {
	directory.readCount++
	directory.lastRequest = count
	if count <= 0 {
		return nil, fmt.Errorf("unbounded ReadDir(%d)", count)
	}
	batchSize := min(count, directory.entryCount-directory.returnedEntries)
	directory.largestBatch = max(directory.largestBatch, batchSize)
	if batchSize == 0 {
		return nil, io.EOF
	}
	children := make([]os.DirEntry, batchSize)
	for index := range children {
		sequence := directory.returnedEntries + index
		name := fmt.Sprintf(".file.sshc-upload-%08d.part", sequence)
		if sequence%2 != 0 {
			name = fmt.Sprintf(".file.sshc-%024x.tmp", sequence)
		}
		child := localComparisonTestEntry{name: name}
		// Cancel on the last name of the first batch, after ReadDir returns.
		if directory.readCount == 1 && index == len(children)-1 {
			child.cancel = directory.cancel
		}
		children[index] = child
	}
	directory.returnedEntries += batchSize
	if directory.eofWithEntries && directory.returnedEntries == directory.entryCount {
		return children, io.EOF
	}
	return children, nil
}

type localComparisonTestEntry struct {
	name   string
	cancel context.CancelFunc
}

func (entry localComparisonTestEntry) Name() string {
	if entry.cancel != nil {
		entry.cancel()
	}
	return entry.name
}

func (entry localComparisonTestEntry) IsDir() bool                { return false }
func (entry localComparisonTestEntry) Type() fs.FileMode          { return 0 }
func (entry localComparisonTestEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

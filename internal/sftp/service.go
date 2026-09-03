package sftp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"sshc/internal/validate"
)

type Service struct {
	Open OpenRemote
	// TemporaryPath はテスト時に差し替える。本番では対象と同じディレクトリへ予測不能な名前を作る。
	TemporaryPath func(target string) (string, error)
}

const (
	previewSniffBytes = 512

	// 検索は「速く終わる」ことを機能の一部として扱う。SFTP の往復は高く、
	// 深い木を全部歩けば数分かかる。予算に当たったら、そこまでの一致と
	// 「まだ続きがある」を返して終わる。
	MaxSearchQueryBytes = 128
	maxSearchResults    = 200
	maxSearchVisited    = 20_000
	maxSearchDepth      = 32

	maxArchiveEntries = 10_000
	maxArchiveDepth   = 64
	maxArchiveBytes   = int64(1 << 30)
	// A regular file transfer is explicitly requested and may be much larger
	// than an archive assembled from an entire directory. Keep a finite safety
	// bound, but do not reuse the 1 GiB archive-expansion budget for files.
	maxRegularFileTransferBytes = int64(512 << 30)
)

type archiveBudget struct {
	entries int
	bytes   int64
}

// PreparedDownload is an immutable local spool of one remote-file read. Its
// revision hashes the exact bytes that will be sent, rather than SFTP v3's
// second-resolution metadata. This prevents equal-size, same-mtime remote
// replacements from being joined to an older downloaded prefix.
type PreparedDownload struct {
	file     *os.File
	name     string
	remove   bool
	lease    *preparedSpoolLease
	Size     int64
	Revision string
}

func (download *PreparedDownload) Close() error {
	if download == nil || download.file == nil {
		return nil
	}
	err := download.file.Close()
	var removeErr error
	if download.remove {
		removeErr = os.Remove(download.name)
	} else if download.lease != nil {
		download.lease.release()
	}
	download.file = nil
	return errors.Join(err, removeErr)
}

type boundedArchiveWriter struct {
	destination io.Writer
	remaining   int64
}

func (writer *boundedArchiveWriter) Write(contents []byte) (int, error) {
	if int64(len(contents)) > writer.remaining {
		return 0, ErrTransferTooLarge
	}
	written, err := writer.destination.Write(contents)
	writer.remaining -= int64(written)
	return written, err
}

func (s Service) prepareArchive(ctx context.Context, alias, remotePath, temporaryDirectory string, maxBytes int64) (_ *PreparedDownload, resultErr error) {
	if maxBytes <= 0 {
		return nil, ErrInvalidTransfer
	}
	temporary, err := os.CreateTemp(temporaryDirectory, "archive-*.part")
	if err != nil {
		return nil, err
	}
	prepared := &PreparedDownload{file: temporary, name: temporary.Name(), remove: true}
	defer func() {
		if resultErr != nil {
			_ = prepared.Close()
		}
	}()
	hash := sha256.New()
	destination := &boundedArchiveWriter{destination: io.MultiWriter(temporary, hash), remaining: maxBytes}
	if _, err := s.DownloadArchive(ctx, alias, remotePath, destination); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	info, err := temporary.Stat()
	if err != nil {
		return nil, err
	}
	prepared.Size = info.Size()
	prepared.Revision = "content-sha256:" + hex.EncodeToString(hash.Sum(nil))
	return prepared, nil
}

func (download *PreparedDownload) WriteFrom(ctx context.Context, offset int64, destination io.Writer) (int64, error) {
	if download == nil || download.file == nil || offset < 0 || offset > download.Size {
		return 0, ErrOffsetMismatch
	}
	if _, err := download.file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	return copyContext(ctx, destination, io.LimitReader(download.file, download.Size-offset), 0)
}

func (s Service) PrepareDownload(ctx context.Context, alias, remotePath string) (_ *PreparedDownload, resultErr error) {
	return s.prepareDownload(ctx, alias, remotePath, "", nil, 0, 1, 0, nil)
}

func (s Service) prepareDownload(
	ctx context.Context, alias, remotePath, temporaryDirectory string, reserve func(int64) error,
	splitThreshold int64, splitParallelism int, splitChunkBytes int64, progress func(DownloadPartProgress),
) (_ *PreparedDownload, resultErr error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return nil, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer remote.Close()
	before, err := remote.Lstat(cleaned)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, ErrNotRegularFile
	}
	if before.Size() < 0 || before.Size() > maxRegularFileTransferBytes {
		return nil, ErrTransferTooLarge
	}
	// Reserve the complete known size before opening or creating a spool. This
	// makes the process-wide disk quota a hard bound even with concurrent jobs.
	if reserve != nil {
		if err := reserve(before.Size()); err != nil {
			return nil, err
		}
	}
	temporary, err := os.CreateTemp(temporaryDirectory, "download-*.part")
	if err != nil {
		return nil, err
	}
	prepared := &PreparedDownload{file: temporary, name: temporary.Name(), remove: true}
	defer func() {
		if resultErr != nil {
			_ = prepared.Close()
		}
	}()
	written := int64(0)
	if before.Size() >= splitThreshold && splitThreshold > 0 && splitParallelism > 1 && splitChunkBytes > 0 {
		if _, supported := remote.(RangeRemote); supported {
			written, err = s.copyDownloadRanges(ctx, remote, alias, cleaned, temporary, before.Size(), splitParallelism, splitChunkBytes, progress)
		}
	}
	if written == 0 && before.Size() > 0 && err == nil {
		written, err = copyDownloadSequential(ctx, remote, cleaned, temporary, before.Size(), progress)
	}
	if err != nil {
		return nil, err
	}
	after, err := remote.Lstat(cleaned)
	if err != nil {
		return nil, err
	}
	if written != before.Size() || metadataRevision(before) != metadataRevision(after) {
		return nil, ErrConflict
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if hashed, err := copyContext(ctx, hash, temporary, 0); err != nil {
		return nil, err
	} else if hashed != written {
		return nil, ErrConflict
	}
	prepared.Size = written
	prepared.Revision = "content-sha256:" + hex.EncodeToString(hash.Sum(nil))
	return prepared, nil
}

func copyDownloadSequential(
	ctx context.Context, remote Remote, remotePath string, destination *os.File, size int64,
	progress func(DownloadPartProgress),
) (int64, error) {
	source, err := remote.Open(remotePath)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	if progress != nil {
		progress(DownloadPartProgress{Index: 0, TotalBytes: size})
	}
	output := io.Writer(destination)
	if progress != nil {
		output = &downloadProgressWriter{destination: destination, progress: func(written int64) {
			progress(DownloadPartProgress{Index: 0, TransferredBytes: written, TotalBytes: size})
		}}
	}
	written, err := copyContext(ctx, output, io.LimitReader(source, size), 0)
	if err != nil {
		return 0, err
	}
	var extra [1]byte
	extraBytes, extraErr := source.Read(extra[:])
	if extraBytes != 0 {
		return 0, ErrConflict
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) {
		return 0, extraErr
	}
	return written, nil
}

type downloadRange struct {
	offset int64
	size   int64
}

// copyDownloadRanges reads non-overlapping ranges over independent SFTP
// connections. The local spool still becomes the single immutable, hashed
// representation used by HTTP retries and browser checkpoints.
func (s Service) copyDownloadRanges(
	ctx context.Context, firstRemote Remote, alias, remotePath string, destination *os.File, size int64, parallelism int, chunkBytes int64,
	progress func(DownloadPartProgress),
) (int64, error) {
	if size <= 0 || parallelism <= 1 || chunkBytes <= 0 {
		return 0, ErrInvalidTransfer
	}
	ranges := make([]downloadRange, 0, int((size+chunkBytes-1)/chunkBytes))
	for offset := int64(0); offset < size; offset += chunkBytes {
		ranges = append(ranges, downloadRange{offset: offset, size: min(chunkBytes, size-offset)})
	}
	if err := destination.Truncate(size); err != nil {
		return 0, err
	}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	workerCount := min(parallelism, len(ranges))
	assignments := make([][]downloadRange, workerCount)
	for index, portion := range ranges {
		worker := index % workerCount
		assignments[worker] = append(assignments[worker], portion)
	}
	writtenParts := make(chan int64, len(ranges))
	var workers sync.WaitGroup
	var firstErr error
	var errorOnce sync.Once
	fail := func(err error) {
		errorOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}
	for workerIndex := range workerCount {
		var workerTotal int64
		for _, portion := range assignments[workerIndex] {
			workerTotal += portion.size
		}
		if progress != nil {
			progress(DownloadPartProgress{Index: workerIndex, TotalBytes: workerTotal})
		}
		workers.Add(1)
		go func(workerIndex int, workerTotal int64) {
			defer workers.Done()
			remote := firstRemote
			if workerIndex != 0 {
				var err error
				remote, err = s.openRequest(workerContext, alias)
				if err != nil {
					fail(err)
					return
				}
				defer remote.Close()
			}
			ranged, ok := remote.(RangeRemote)
			if !ok {
				fail(ErrInvalidTransfer)
				return
			}
			var workerWritten int64
			for _, portion := range assignments[workerIndex] {
				last := portion.offset+portion.size == size
				written, err := copyDownloadRange(workerContext, ranged, remotePath, destination, portion, last, func(chunkWritten int64) {
					if progress != nil {
						progress(DownloadPartProgress{Index: workerIndex, TransferredBytes: workerWritten + chunkWritten, TotalBytes: workerTotal})
					}
				})
				if err != nil {
					fail(err)
					return
				}
				workerWritten += written
				writtenParts <- written
			}
		}(workerIndex, workerTotal)
	}
	workers.Wait()
	close(writtenParts)
	if firstErr != nil {
		return 0, firstErr
	}
	var written int64
	for part := range writtenParts {
		written += part
	}
	return written, nil
}

func copyDownloadRange(
	ctx context.Context, remote RangeRemote, remotePath string, destination *os.File, portion downloadRange, last bool,
	progress func(int64),
) (int64, error) {
	source, err := remote.OpenRange(remotePath, portion.offset)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	output := io.Writer(io.NewOffsetWriter(destination, portion.offset))
	if progress != nil {
		output = &downloadProgressWriter{destination: output, progress: progress}
	}
	written, err := copyContext(ctx, output, io.LimitReader(source, portion.size), 0)
	if err != nil {
		return written, err
	}
	if written != portion.size {
		return written, ErrConflict
	}
	if last {
		var extra [1]byte
		if count, readErr := source.Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			return written, ErrConflict
		}
	}
	return written, nil
}

type downloadProgressWriter struct {
	destination io.Writer
	written     int64
	progress    func(int64)
}

func (writer *downloadProgressWriter) Write(contents []byte) (int, error) {
	written, err := writer.destination.Write(contents)
	writer.written += int64(written)
	writer.progress(writer.written)
	return written, err
}

func (s Service) ListDirectory(ctx context.Context, alias, remotePath string) (Listing, error) {
	var cleaned string
	var err error
	if remotePath != "" {
		cleaned, err = cleanPublicPath(remotePath, true)
		if err != nil {
			return Listing{}, err
		}
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Listing{}, err
	}
	defer remote.Close()
	if remotePath == "" {
		workingDirectory, err := remote.Getwd(ctx)
		if err != nil {
			return Listing{}, err
		}
		cleaned, err = cleanPublicPath(workingDirectory, true)
		if err != nil {
			return Listing{}, err
		}
	}

	infos, err := remote.ReadDir(ctx, cleaned)
	if err != nil {
		return Listing{}, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		if isInternalName(info.Name()) {
			continue
		}
		entries = append(entries, entryFrom(cleaned, info))
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Type == EntryDirectory && entries[right].Type != EntryDirectory {
			return true
		}
		if entries[left].Type != EntryDirectory && entries[right].Type == EntryDirectory {
			return false
		}
		leftName, rightName := strings.ToLower(entries[left].Name), strings.ToLower(entries[right].Name)
		if leftName == rightName {
			return entries[left].Name < entries[right].Name
		}
		return leftName < rightName
	})
	return Listing{Path: cleaned, Entries: entries}, nil
}

// Search は、あるディレクトリ配下から名前に query を含む項目を集める。
//
// symlink は辿らない。辿れば輪に入りうるし、同じ実体を別の名前で二度返す。
func (s Service) Search(ctx context.Context, alias, remotePath, query string) (SearchResult, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" || len(needle) > MaxSearchQueryBytes {
		return SearchResult{}, ErrInvalidQuery
	}
	root, err := cleanPublicPath(remotePath, true)
	if err != nil {
		return SearchResult{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return SearchResult{}, err
	}
	defer remote.Close()

	result := SearchResult{Path: root, Query: query}
	visited := 0
	pending := []string{root}
	for depth := 0; depth <= maxSearchDepth && len(pending) > 0; depth++ {
		var next []string
		for _, directory := range pending {
			if err := ctx.Err(); err != nil {
				return SearchResult{}, err
			}
			infos, err := remote.ReadDir(ctx, directory)
			if err != nil {
				// 読めない枝は飛ばす。権限のない一つのディレクトリで検索
				// 全体を落とすほうが、利用者にとって役に立たない。
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return SearchResult{}, err
				}
				result.Truncated = true
				continue
			}
			for _, info := range infos {
				if isInternalName(info.Name()) {
					continue
				}
				visited++
				if visited > maxSearchVisited {
					result.Truncated = true
					return result, nil
				}
				entry := entryFrom(directory, info)
				if strings.Contains(strings.ToLower(entry.Name), needle) {
					if len(result.Entries) >= maxSearchResults {
						result.Truncated = true
						return result, nil
					}
					result.Entries = append(result.Entries, entry)
				}
				if entry.Type == EntryDirectory {
					next = append(next, entry.Path)
				}
			}
		}
		if depth == maxSearchDepth && len(next) > 0 {
			result.Truncated = true
			break
		}
		pending = next
	}
	return result, nil
}

func (s Service) List(ctx context.Context, alias, remotePath string) ([]Entry, error) {
	listing, err := s.ListDirectory(ctx, alias, remotePath)
	return listing.Entries, err
}

func (s Service) Stat(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPublicPath(remotePath, true)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
}

func (s Service) ReadText(ctx context.Context, alias, remotePath string) (TextFile, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()
	return readText(ctx, remote, cleaned)
}

// previewContentType は、先頭のバイト列だけからその型を返す。preview で
// 描いてよい型でなければ空を返す。名前が名乗る型は使わない。拡張子は
// 中身について何も保証しないからで、background 画像と同じ扱いである。
//
// 画像だけである。PDF は <iframe> でしか描けず、それには CSP へ blob: を
// 足す必要がある。画像は data: URL で足りるので、policy はそのままでよい。
//
// SVG もここに無い。<img> の中では script が動かないとはいえ、中身から
// 型が決まらない唯一の候補であり、background service も同じ理由で断る。
func previewContentType(head []byte) string {
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "image/jpeg"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "image/gif"
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return "image/bmp"
	}
	return ""
}

// ReadPreview は、詳細モーダルが描ける画像を丸ごと返す。
//
// 型が分かるまでに読むのは先頭 previewSniffBytes だけである。preview に
// できない大きな書庫を、断ると分かっている間ずっと転送し続けない。
func (s Service) ReadPreview(ctx context.Context, alias, remotePath string) (Preview, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Preview{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Preview{}, err
	}
	defer remote.Close()
	before, err := remote.Lstat(cleaned)
	if err != nil {
		return Preview{}, err
	}
	if !before.Mode().IsRegular() {
		return Preview{}, ErrNotRegularFile
	}
	if before.Size() > MaxPreviewFileBytes {
		return Preview{}, ErrPreviewTooLarge
	}
	contents, contentType, err := readPreviewBytes(ctx, remote, cleaned)
	if err != nil {
		return Preview{}, err
	}
	after, err := remote.Lstat(cleaned)
	if err != nil {
		return Preview{}, err
	}
	// 読んでいるあいだに置き換わったものを、古い metadata のまま見せない。
	if metadataRevision(before) != metadataRevision(after) {
		return Preview{}, ErrConflict
	}
	entry := entryFrom(path.Dir(cleaned), namedInfo{FileInfo: after, name: path.Base(cleaned)})
	return Preview{Entry: entry, ContentType: contentType, Contents: contents, Revision: contentRevision(after, contents)}, nil
}

func readPreviewBytes(ctx context.Context, remote Remote, cleaned string) ([]byte, string, error) {
	file, err := remote.Open(cleaned)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = file.Close() }()
	reader := &contextReader{ctx: ctx, reader: file}
	head := make([]byte, previewSniffBytes)
	read, err := io.ReadFull(reader, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, "", err
	}
	head = head[:read]
	contentType := previewContentType(head)
	if contentType == "" {
		return nil, "", ErrPreviewType
	}
	rest, err := io.ReadAll(io.LimitReader(reader, MaxPreviewFileBytes+1-int64(read)))
	if err != nil {
		return nil, "", err
	}
	contents := append(head, rest...)
	if len(contents) > MaxPreviewFileBytes {
		return nil, "", ErrPreviewTooLarge
	}
	return contents, contentType, nil
}

func (s Service) SaveText(
	ctx context.Context, alias, remotePath, contents, expectedRevision string,
) (TextFile, error) {
	if expectedRevision == "" {
		return TextFile{}, ErrRevisionRequired
	}
	if len(contents) > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	if !validText([]byte(contents)) {
		return TextFile{}, ErrNotUTF8
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()

	current, err := readText(ctx, remote, cleaned)
	if err != nil {
		return TextFile{}, err
	}
	if current.Revision != expectedRevision {
		return TextFile{}, ErrConflict
	}
	verify := func() error {
		latest, err := readText(ctx, remote, cleaned)
		if errors.Is(err, fs.ErrNotExist) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		if latest.Revision != expectedRevision {
			return ErrConflict
		}
		return nil
	}
	if _, err := s.replace(
		ctx, remote, cleaned, strings.NewReader(contents), current.Entry.Mode.Perm(), 0, verify,
	); err != nil {
		return TextFile{}, err
	}
	return readText(ctx, remote, cleaned)
}

// DownloadArchive streams a directory as a ZIP without following symlinks.
// Symlinks become regular text entries containing the link target so extraction cannot escape via a link.
func (s Service) DownloadArchive(ctx context.Context, alias, remotePath string, destination io.Writer) (Transfer, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Transfer{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Transfer{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Transfer{}, err
	}
	if !info.IsDir() {
		return Transfer{}, ErrNotDirectory
	}
	rootName := path.Base(cleaned)
	if !validArchiveName(rootName) {
		return Transfer{}, ErrInvalidPath
	}
	archive := zip.NewWriter(destination)
	var written int64
	budget := &archiveBudget{entries: 1}
	if err := archiveDirectory(ctx, archive, remote, cleaned, rootName, 1, budget, &written); err != nil {
		_ = archive.Close()
		return Transfer{}, err
	}
	if err := archive.Close(); err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: written, Revision: metadataRevision(info)}, nil
}

func archiveDirectory(
	ctx context.Context, archive *zip.Writer, remote Remote, directory, archivePath string,
	depth int, budget *archiveBudget, written *int64,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	header := &zip.FileHeader{Name: strings.TrimSuffix(archivePath, "/") + "/", Method: zip.Store}
	header.SetMode(fs.ModeDir | 0o755)
	if _, err := archive.CreateHeader(header); err != nil {
		return err
	}
	infos, err := remote.ReadDir(ctx, directory)
	if err != nil {
		return err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	for _, info := range infos {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isInternalName(info.Name()) {
			continue
		}
		if !validArchiveName(info.Name()) {
			return ErrInvalidPath
		}
		budget.entries++
		if budget.entries > maxArchiveEntries {
			return ErrTransferTooLarge
		}
		remoteChild := path.Join(directory, info.Name())
		archiveChild := path.Join(archivePath, info.Name())
		switch {
		case info.IsDir():
			if depth >= maxArchiveDepth {
				return ErrTransferTooLarge
			}
			if err := archiveDirectory(ctx, archive, remote, remoteChild, archiveChild, depth+1, budget, written); err != nil {
				return err
			}
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := remote.ReadLink(remoteChild)
			if err != nil {
				return err
			}
			if int64(len(target)) > maxArchiveBytes-budget.bytes {
				return ErrTransferTooLarge
			}
			budget.bytes += int64(len(target))
			// Materialize the target as a regular text entry. Creating an actual
			// symlink in an archive can escape the extraction directory.
			header := &zip.FileHeader{Name: archiveChild, Method: zip.Store}
			header.SetMode(0o600)
			entry, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			count, err := io.WriteString(entry, target)
			*written += int64(count)
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			available := maxArchiveBytes - budget.bytes
			if info.Size() < 0 || info.Size() > available {
				return ErrTransferTooLarge
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name, header.Method = archiveChild, zip.Deflate
			entry, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			file, err := remote.Open(remoteChild)
			if err != nil {
				return err
			}
			count, copyErr := copyContext(ctx, entry, io.LimitReader(file, available+1), 0)
			closeErr := file.Close()
			*written += count
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if count > available {
				return ErrTransferTooLarge
			}
			if count != info.Size() {
				return ErrConflict
			}
			budget.bytes += count
		}
	}
	return nil
}

func validArchiveName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func (s Service) Mkdir(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	if mkdirErr := remote.Mkdir(cleaned); mkdirErr != nil {
		info, statErr := remote.Lstat(cleaned)
		if statErr != nil {
			return Entry{}, mkdirErr
		}
		if !info.IsDir() {
			return Entry{}, ErrAlreadyExists
		}
		return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
	}
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
}

func (s Service) Chmod(ctx context.Context, alias, remotePath string, mode fs.FileMode, expectedRevision string) (Entry, error) {
	if expectedRevision == "" {
		return Entry{}, ErrRevisionRequired
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return Entry{}, ErrNotRegularFile
	}
	if metadataRevision(info) != expectedRevision {
		return Entry{}, ErrConflict
	}
	if err := remote.Chmod(cleaned, mode.Perm()); err != nil {
		return Entry{}, err
	}
	updated, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: updated, name: path.Base(cleaned)}), nil
}

// Rename は既存の移動先を上書きしない。置換は Upload と SaveText だけが明示的に扱う。
func (s Service) Rename(ctx context.Context, alias, from, to string) (Entry, error) {
	source, err := cleanPublicPath(from, false)
	if err != nil {
		return Entry{}, err
	}
	target, err := cleanPublicPath(to, false)
	if err != nil {
		return Entry{}, err
	}
	if source == target {
		return Entry{}, ErrAlreadyExists
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	if _, statErr := remote.Lstat(target); statErr == nil {
		return Entry{}, ErrAlreadyExists
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return Entry{}, statErr
	}
	if err := remote.Rename(source, target); err != nil {
		return Entry{}, err
	}
	info, err := remote.Lstat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(target), namedInfo{FileInfo: info, name: path.Base(target)}), nil
}

// Delete はファイル、symlink、空ディレクトリだけを削除する。再帰削除は提供しない。
func (s Service) Delete(ctx context.Context, alias, remotePath string) error {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return remote.RemoveDirectory(cleaned)
	}
	return remote.Remove(cleaned)
}

func (s Service) open(ctx context.Context, alias string) (Remote, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	if s.Open == nil {
		return nil, ErrUnavailable
	}
	return s.Open(ctx, alias)
}

// openRequest binds an operation-scoped remote to the request context. Closing
// the SFTP transport is the only portable way to wake a pkg/sftp Read or Write
// which is already blocked when its context is cancelled.
func (s Service) openRequest(ctx context.Context, alias string) (Remote, error) {
	remote, err := s.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	return bindRemoteContext(ctx, remote), nil
}

func (s Service) replace(
	ctx context.Context,
	remote Remote,
	target string,
	source io.Reader,
	mode fs.FileMode,
	maxBytes int64,
	verify func() error,
) (written int64, resultErr error) {
	temporary, err := s.temporaryPath(target)
	if err != nil {
		return 0, err
	}
	if path.Dir(temporary) != path.Dir(target) || temporary == target {
		return 0, fmt.Errorf("%w: temporary path must be beside the target", ErrInvalidPath)
	}
	file, err := remote.Create(temporary)
	if err != nil {
		return 0, err
	}
	cleanup := true
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
		if cleanup {
			_ = remote.Remove(temporary)
		}
	}()

	written, err = copyContext(ctx, file, source, maxBytes)
	if err != nil {
		return written, err
	}
	closeErr := file.Close()
	file = closedWriter{}
	if closeErr != nil {
		return written, closeErr
	}
	if err := remote.Chmod(temporary, mode.Perm()); err != nil {
		return written, err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return written, err
		}
	}
	if err := remote.Replace(temporary, target); err != nil {
		return written, err
	}
	cleanup = false
	return written, nil
}

func (s Service) temporaryPath(target string) (string, error) {
	if s.TemporaryPath != nil {
		return s.TemporaryPath(target)
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return path.Join(path.Dir(target), "."+path.Base(target)+".sshc-"+hex.EncodeToString(suffix[:])+".tmp"), nil
}

func readText(ctx context.Context, remote Remote, cleaned string) (TextFile, error) {
	before, err := remote.Lstat(cleaned)
	if err != nil {
		return TextFile{}, err
	}
	if !before.Mode().IsRegular() {
		return TextFile{}, ErrNotRegularFile
	}
	if before.Size() > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	file, err := remote.Open(cleaned)
	if err != nil {
		return TextFile{}, err
	}
	contents, readErr := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: file}, MaxEditableFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return TextFile{}, readErr
	}
	if closeErr != nil {
		return TextFile{}, closeErr
	}
	if len(contents) > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	if !validText(contents) {
		return TextFile{}, ErrNotUTF8
	}
	after, err := remote.Lstat(cleaned)
	if err != nil {
		return TextFile{}, err
	}
	if metadataRevision(before) != metadataRevision(after) {
		return TextFile{}, ErrConflict
	}
	entry := entryFrom(path.Dir(cleaned), namedInfo{FileInfo: after, name: path.Base(cleaned)})
	return TextFile{Entry: entry, Contents: string(contents), Revision: contentRevision(after, contents)}, nil
}

func cleanPath(candidate string, allowRoot bool) (string, error) {
	if candidate == "" || strings.IndexByte(candidate, 0) >= 0 || !path.IsAbs(candidate) {
		return "", ErrInvalidPath
	}
	cleaned := path.Clean(candidate)
	if !allowRoot && cleaned == "/" {
		return "", ErrRootOperation
	}
	return cleaned, nil
}

func cleanPublicPath(candidate string, allowRoot bool) (string, error) {
	cleaned, err := cleanPath(candidate, allowRoot)
	if err != nil {
		return "", err
	}
	for _, segment := range strings.Split(strings.TrimPrefix(cleaned, "/"), "/") {
		if isInternalName(segment) {
			return "", ErrInvalidPath
		}
	}
	return cleaned, nil
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return ErrInvalidAlias
	}
	return validate.Alias(alias)
}

type contextRemote struct {
	Remote
	once     sync.Once
	mutex    sync.Mutex
	stop     func() bool
	closed   bool
	closeErr error
}

func bindRemoteContext(ctx context.Context, remote Remote) Remote {
	bound := &contextRemote{Remote: remote}
	stop := context.AfterFunc(ctx, func() { _ = bound.Close() })
	bound.mutex.Lock()
	bound.stop = stop
	closed := bound.closed
	bound.mutex.Unlock()
	if closed {
		stop()
	}
	if _, ok := remote.(RangeRemote); ok {
		return &contextRangeRemote{contextRemote: bound}
	}
	return bound
}

type contextRangeRemote struct{ *contextRemote }

func (remote *contextRangeRemote) OpenRange(candidate string, offset int64) (io.ReadCloser, error) {
	ranged, ok := remote.Remote.(RangeRemote)
	if !ok {
		return nil, ErrInvalidTransfer
	}
	return ranged.OpenRange(candidate, offset)
}

func (remote *contextRemote) Close() error {
	remote.once.Do(func() {
		remote.mutex.Lock()
		remote.closed = true
		stop := remote.stop
		remote.mutex.Unlock()
		if stop != nil {
			stop()
		}
		remote.closeErr = remote.Remote.Close()
	})
	return remote.closeErr
}

func entryFrom(parent string, info fs.FileInfo) Entry {
	typeOf := EntryOther
	switch {
	case info.IsDir():
		typeOf = EntryDirectory
	case info.Mode().IsRegular():
		typeOf = EntryFile
	case info.Mode()&fs.ModeSymlink != 0:
		typeOf = EntrySymlink
	}
	return Entry{
		Name:       info.Name(),
		Path:       path.Join(parent, info.Name()),
		Type:       typeOf,
		Size:       info.Size(),
		Mode:       info.Mode(),
		ModifiedAt: info.ModTime().UTC(),
		Revision:   metadataRevision(info),
	}
}

func metadataRevision(info fs.FileInfo) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%d\x00%d\x00", info.Size(), info.Mode(), info.ModTime().UTC().UnixNano())
	return "meta-sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func contentRevision(info fs.FileInfo, contents []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, metadataRevision(info))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(contents)
	return "content-sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func validText(contents []byte) bool {
	return utf8.Valid(contents) && !bytes.ContainsRune(contents, '\x00')
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader, maxBytes int64) (int64, error) {
	reader := io.Reader(&contextReader{ctx: ctx, reader: source})
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	written, err := io.Copy(destination, reader)
	if err != nil {
		return written, err
	}
	if maxBytes > 0 && written > maxBytes {
		return written, ErrTransferTooLarge
	}
	return written, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(contents []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		read, err := r.reader.Read(contents)
		if err != nil && r.ctx.Err() != nil {
			return read, r.ctx.Err()
		}
		return read, err
	}
}

type namedInfo struct {
	fs.FileInfo
	name string
}

func (i namedInfo) Name() string { return i.name }

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, fs.ErrClosed }
func (closedWriter) Close() error              { return nil }

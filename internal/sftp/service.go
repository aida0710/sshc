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
	"unicode/utf8"
)

type Service struct {
	Open OpenRemote
	// TemporaryPath はテスト時に差し替える。本番では対象と同じディレクトリへ予測不能な名前を作る。
	TemporaryPath func(target string) (string, error)
}

const (
	maxArchiveEntries = 10_000
	maxArchiveDepth   = 64
	maxArchiveBytes   = int64(1 << 30)
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
	return s.prepareDownload(ctx, alias, remotePath, "", nil)
}

func (s Service) prepareDownload(ctx context.Context, alias, remotePath, temporaryDirectory string, reserve func(int64) error) (_ *PreparedDownload, resultErr error) {
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return nil, err
	}
	remote, err := s.open(ctx, alias)
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
	if before.Size() < 0 || before.Size() > maxArchiveBytes {
		return nil, ErrTransferTooLarge
	}
	// Reserve the complete known size before opening or creating a spool. This
	// makes the process-wide disk quota a hard bound even with concurrent jobs.
	if reserve != nil {
		if err := reserve(before.Size()); err != nil {
			return nil, err
		}
	}
	source, err := remote.Open(cleaned)
	if err != nil {
		return nil, err
	}
	defer source.Close()
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
	hash := sha256.New()
	// The manager reserved exactly before.Size() bytes. Limit the open handle to
	// that amount and probe one extra byte without writing it, so a growing
	// remote file can never make the disk spool exceed the reservation.
	written, err := copyContext(ctx, io.MultiWriter(temporary, hash), io.LimitReader(source, before.Size()), 0)
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	extraBytes, extraErr := source.Read(extra[:])
	if extraBytes != 0 {
		return nil, ErrConflict
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) {
		return nil, extraErr
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
	prepared.Size = written
	prepared.Revision = "content-sha256:" + hex.EncodeToString(hash.Sum(nil))
	return prepared, nil
}

func (s Service) List(ctx context.Context, alias, remotePath string) ([]Entry, error) {
	cleaned, err := cleanPath(remotePath, true)
	if err != nil {
		return nil, err
	}
	remote, err := s.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer remote.Close()

	infos, err := remote.ReadDir(ctx, cleaned)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		if isUploadPartName(info.Name()) {
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
	return entries, nil
}

func (s Service) Stat(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPath(remotePath, true)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.open(ctx, alias)
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.open(ctx, alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()
	return readText(ctx, remote, cleaned)
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.open(ctx, alias)
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return Transfer{}, err
	}
	remote, err := s.open(ctx, alias)
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
		if isUploadPartName(info.Name()) {
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.open(ctx, alias)
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.open(ctx, alias)
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
	source, err := cleanPath(from, false)
	if err != nil {
		return Entry{}, err
	}
	target, err := cleanPath(to, false)
	if err != nil {
		return Entry{}, err
	}
	if source == target {
		return Entry{}, ErrAlreadyExists
	}
	remote, err := s.open(ctx, alias)
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
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return err
	}
	remote, err := s.open(ctx, alias)
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
	if strings.TrimSpace(alias) == "" {
		return nil, ErrInvalidAlias
	}
	if s.Open == nil {
		return nil, ErrUnavailable
	}
	return s.Open(ctx, alias)
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
		return r.reader.Read(contents)
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

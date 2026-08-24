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

func (s Service) Download(ctx context.Context, alias, remotePath string, destination io.Writer) (Transfer, error) {
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
	if !info.Mode().IsRegular() {
		return Transfer{}, ErrNotRegularFile
	}
	file, err := remote.Open(cleaned)
	if err != nil {
		return Transfer{}, err
	}
	defer file.Close()
	written, err := copyContext(ctx, destination, file, 0)
	if err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: written, Revision: metadataRevision(info)}, nil
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
	if err := archiveDirectory(ctx, archive, remote, cleaned, rootName, &written); err != nil {
		_ = archive.Close()
		return Transfer{}, err
	}
	if err := archive.Close(); err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: written, Revision: metadataRevision(info)}, nil
}

func archiveDirectory(ctx context.Context, archive *zip.Writer, remote Remote, directory, archivePath string, written *int64) error {
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
		if !validArchiveName(info.Name()) {
			return ErrInvalidPath
		}
		remoteChild := path.Join(directory, info.Name())
		archiveChild := path.Join(archivePath, info.Name())
		switch {
		case info.IsDir():
			if err := archiveDirectory(ctx, archive, remote, remoteChild, archiveChild, written); err != nil {
				return err
			}
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := remote.ReadLink(remoteChild)
			if err != nil {
				return err
			}
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
			count, copyErr := copyContext(ctx, entry, file, 0)
			closeErr := file.Close()
			*written += count
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

func validArchiveName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func (s Service) Upload(
	ctx context.Context, alias, remotePath string, source io.Reader, options UploadOptions,
) (Transfer, error) {
	cleaned, err := cleanPath(remotePath, false)
	if err != nil {
		return Transfer{}, err
	}
	if options.MaxBytes < 0 {
		return Transfer{}, ErrTransferTooLarge
	}
	remote, err := s.open(ctx, alias)
	if err != nil {
		return Transfer{}, err
	}
	defer remote.Close()

	mode := options.Mode.Perm()
	info, statErr := remote.Lstat(cleaned)
	switch {
	case statErr == nil:
		if !info.Mode().IsRegular() {
			return Transfer{}, ErrNotRegularFile
		}
		if !options.Overwrite {
			return Transfer{}, ErrAlreadyExists
		}
		if options.ExpectedRevision != "" && metadataRevision(info) != options.ExpectedRevision {
			return Transfer{}, ErrConflict
		}
		mode = info.Mode().Perm()
	case !errors.Is(statErr, fs.ErrNotExist):
		return Transfer{}, statErr
	case options.ExpectedRevision != "":
		return Transfer{}, ErrConflict
	}
	if mode == 0 {
		mode = 0o600
	}
	var verify func() error
	if statErr == nil && options.ExpectedRevision != "" {
		verify = func() error {
			latest, err := remote.Lstat(cleaned)
			if err != nil || metadataRevision(latest) != options.ExpectedRevision {
				return ErrConflict
			}
			return nil
		}
	} else if errors.Is(statErr, fs.ErrNotExist) {
		verify = func() error {
			if _, err := remote.Lstat(cleaned); errors.Is(err, fs.ErrNotExist) {
				return nil
			} else if err != nil {
				return err
			}
			return ErrAlreadyExists
		}
	}
	written, err := s.replace(ctx, remote, cleaned, source, mode, options.MaxBytes, verify)
	if err != nil {
		return Transfer{}, err
	}
	updated, err := remote.Lstat(cleaned)
	if err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: written, Revision: metadataRevision(updated)}, nil
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
	if err := remote.Mkdir(cleaned); err != nil {
		return Entry{}, err
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

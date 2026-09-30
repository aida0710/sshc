package sftp

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Directory downloads as a ZIP stream, with the entry, depth and byte
// budgets that keep an unexpectedly large tree from running away.

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
	if !ValidLocalChildName(rootName) {
		return Transfer{}, ErrInvalidPath
	}
	walk := &archiveWalk{archive: zip.NewWriter(destination), remote: remote, budget: archiveBudget{entries: 1}}
	if err := walk.addDirectory(ctx, archiveItem{remotePath: cleaned, archivePath: rootName, depth: 1}); err != nil {
		_ = walk.archive.Close()
		return Transfer{}, err
	}
	if err := walk.archive.Close(); err != nil {
		return Transfer{}, err
	}
	return Transfer{Path: cleaned, Bytes: walk.written, Revision: metadataRevision(info)}, nil
}

// archiveWalk is the state of writing one ZIP. The budget counts the whole
// tree, not one folder.
type archiveWalk struct {
	archive *zip.Writer
	remote  Remote
	budget  archiveBudget
	written int64
}

// archiveItem is one entry to add: where it is on the remote and what it is
// called inside the ZIP.
type archiveItem struct {
	remotePath  string
	archivePath string
	// depth counts the root folder as 1.
	depth int
}

func (w *archiveWalk) addDirectory(ctx context.Context, directory archiveItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	header := &zip.FileHeader{Name: strings.TrimSuffix(directory.archivePath, "/") + "/", Method: zip.Store}
	header.SetMode(fs.ModeDir | 0o755)
	if _, err := w.archive.CreateHeader(header); err != nil {
		return err
	}
	infos, err := readChildren(ctx, w.remote, directory.remotePath)
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
		if !ValidLocalChildName(info.Name()) {
			return ErrInvalidPath
		}
		w.budget.entries++
		if w.budget.entries > maxArchiveEntries {
			return ErrTransferTooLarge
		}
		child := archiveItem{
			remotePath:  path.Join(directory.remotePath, info.Name()),
			archivePath: path.Join(directory.archivePath, info.Name()),
			depth:       directory.depth + 1,
		}
		switch {
		case info.IsDir():
			if directory.depth >= maxArchiveDepth {
				return ErrTransferTooLarge
			}
			err = w.addDirectory(ctx, child)
		case info.Mode()&fs.ModeSymlink != 0:
			err = w.addLink(child)
		case info.Mode().IsRegular():
			err = w.addFile(ctx, child, info)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// addLink materializes the link target as a regular text entry. Creating an
// actual symlink in an archive can escape the extraction directory.
func (w *archiveWalk) addLink(link archiveItem) error {
	target, err := w.remote.ReadLink(link.remotePath)
	if err != nil {
		return err
	}
	if int64(len(target)) > maxArchiveBytes-w.budget.bytes {
		return ErrTransferTooLarge
	}
	w.budget.bytes += int64(len(target))
	header := &zip.FileHeader{Name: link.archivePath, Method: zip.Store}
	header.SetMode(0o600)
	entry, err := w.archive.CreateHeader(header)
	if err != nil {
		return err
	}
	count, err := io.WriteString(entry, target)
	w.written += int64(count)
	return err
}

func (w *archiveWalk) addFile(ctx context.Context, file archiveItem, info fs.FileInfo) error {
	available := maxArchiveBytes - w.budget.bytes
	if info.Size() < 0 || info.Size() > available {
		return ErrTransferTooLarge
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name, header.Method = file.archivePath, zip.Deflate
	entry, err := w.archive.CreateHeader(header)
	if err != nil {
		return err
	}
	source, err := w.remote.Open(file.remotePath)
	if err != nil {
		return err
	}
	count, copyErr := copyContext(ctx, entry, io.LimitReader(source, available+1), 0)
	closeErr := source.Close()
	w.written += count
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
	w.budget.bytes += count
	return nil
}

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

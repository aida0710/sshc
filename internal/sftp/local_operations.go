package sftp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Local paths name the engine filesystem, never the browser filesystem. The
// entries share Entry with remote listings so that one file list can show
// either side with the same columns.
type LocalListing struct {
	Path    string  `json:"path"`
	Home    string  `json:"home"`
	Entries []Entry `json:"entries"`
}

func localRelative(value string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if value == "" || value == "~" || value == "~/" {
		value = home
	} else if strings.HasPrefix(value, "~/") {
		value = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(value, "~/")))
	}
	if strings.ContainsRune(value, 0) || !filepath.IsAbs(value) {
		return "", ErrInvalidPath
	}
	cleaned := filepath.Clean(value)
	withoutTrailingSlash := strings.TrimSuffix(strings.TrimSuffix(value, "/"), string(filepath.Separator))
	if cleaned != value && value != filepath.ToSlash(cleaned) &&
		withoutTrailingSlash != cleaned && withoutTrailingSlash != filepath.ToSlash(cleaned) {
		return "", ErrInvalidPath
	}
	return filepath.ToSlash(cleaned), nil
}
func localPublic(relative string, root *os.Root) string {
	return filepath.ToSlash(filepath.Join(root.Name(), filepath.FromSlash(relative)))
}
func openLocalRoot(value string) (*os.Root, string, error) {
	absolute, err := localRelative(value)
	if err != nil {
		return nil, "", err
	}
	volume := filepath.VolumeName(filepath.FromSlash(absolute))
	filesystemRoot := volume + string(filepath.Separator)
	root, err := os.OpenRoot(filesystemRoot)
	if err != nil {
		return nil, "", err
	}
	relative, err := filepath.Rel(filesystemRoot, filepath.FromSlash(absolute))
	if err != nil {
		root.Close()
		return nil, "", err
	}
	return root, filepath.ToSlash(relative), nil
}
func checkLocal(root *os.Root, relative string, allowMissing bool) (fs.FileInfo, error) {
	if relative == "." {
		return root.Lstat(".")
	}
	parts := strings.Split(relative, "/")
	for index := range parts {
		current := path.Join(parts[:index+1]...)
		var info fs.FileInfo
		var err error
		if index < len(parts)-1 {
			info, err = root.Stat(current)
		} else {
			info, err = root.Lstat(current)
		}
		if err != nil {
			if allowMissing && index == len(parts)-1 && errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrUnsupportedEntry
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, ErrNotDirectory
		}
		if index == len(parts)-1 {
			return info, nil
		}
	}
	return nil, ErrInvalidPath
}
func ListLocal(value string) (LocalListing, error) {
	root, relative, err := openLocalRoot(value)
	if err != nil {
		return LocalListing{}, err
	}
	defer root.Close()
	info, err := root.Stat(relative)
	if err != nil {
		return LocalListing{}, err
	}
	if !info.IsDir() {
		return LocalListing{}, ErrNotDirectory
	}
	dir, err := root.Open(relative)
	if err != nil {
		return LocalListing{}, err
	}
	defer dir.Close()
	infos, err := dir.Readdir(0)
	if err != nil {
		return LocalListing{}, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, item := range infos {
		name := item.Name()
		if item.Mode()&fs.ModeSymlink != 0 {
			resolved, resolveErr := root.Stat(path.Join(relative, name))
			if resolveErr != nil {
				continue
			}
			item = resolved
		}
		if !item.Mode().IsRegular() && !item.IsDir() {
			continue
		}
		entry := entryFrom("", item)
		entry.Name = name
		entry.Path = localPublic(path.Join(relative, name), root)
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == EntryDirectory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	home, err := os.UserHomeDir()
	if err != nil {
		return LocalListing{}, err
	}
	return LocalListing{Path: localPublic(relative, root), Home: filepath.ToSlash(home), Entries: entries}, nil
}

func (s Service) PlanLocalTransfer(ctx context.Context, request RemoteTransferRequest) (RemoteTransferPlan, error) {
	var localPath, remotePath string
	if request.Operation == RemotePut {
		localPath, remotePath = request.SourcePath, request.TargetPath
	} else if request.Operation == RemoteGet {
		localPath, remotePath = request.TargetPath, request.SourcePath
	} else {
		return RemoteTransferPlan{}, ErrInvalidTransfer
	}
	if _, err := localRelative(localPath); err != nil {
		return RemoteTransferPlan{}, err
	}
	if _, err := cleanPublicPath(remotePath, false); err != nil {
		return RemoteTransferPlan{}, err
	}
	if request.Operation == RemotePut {
		root, relative, err := openLocalRoot(localPath)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		defer root.Close()
		info, err := checkLocal(root, relative, false)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		total, err := localTreeBytes(ctx, root, relative, info)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		kind := TransferFile
		if info.IsDir() {
			kind = TransferFolder
		}
		return RemoteTransferPlan{Name: path.Base(relative), Kind: kind, TotalBytes: total}, nil
	}
	remote, err := s.openRequest(ctx, request.SourceAlias)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(remotePath)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	total := info.Size()
	kind := TransferFile
	if info.IsDir() {
		kind = TransferFolder
		total, err = treeBytes(ctx, remote, remotePath)
	} else if !info.Mode().IsRegular() {
		return RemoteTransferPlan{}, ErrUnsupportedEntry
	}
	return RemoteTransferPlan{Name: path.Base(remotePath), Kind: kind, TotalBytes: total}, err
}
func localTreeBytes(ctx context.Context, root *os.Root, relative string, info fs.FileInfo) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if info.Mode().IsRegular() {
		return info.Size(), nil
	}
	if !info.IsDir() {
		return 0, ErrUnsupportedEntry
	}
	dir, err := root.Open(relative)
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	infos, err := dir.Readdir(0)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, child := range infos {
		childRelative := path.Join(relative, child.Name())
		checked, err := checkLocal(root, childRelative, false)
		if err != nil {
			return 0, err
		}
		size, err := localTreeBytes(ctx, root, childRelative, checked)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}
func (s Service) CopyLocal(ctx context.Context, request RemoteTransferRequest, progress func(int64) error) error {
	if request.Operation != RemoteGet && request.Operation != RemotePut {
		return ErrInvalidTransfer
	}
	var localPath string
	if request.Operation == RemotePut {
		localPath = request.SourcePath
	} else {
		localPath = request.TargetPath
	}
	root, relative, err := openLocalRoot(localPath)
	if err != nil {
		return err
	}
	defer root.Close()
	var transferred int64
	report := func(delta int64) error {
		transferred += delta
		if progress != nil {
			return progress(transferred)
		}
		return nil
	}
	if request.Operation == RemotePut {
		sourceInfo, err := checkLocal(root, relative, false)
		if err != nil {
			return err
		}
		remote, err := s.openRequest(ctx, request.TargetAlias)
		if err != nil {
			return err
		}
		defer remote.Close()
		return s.putLocal(ctx, root, remote, relative, request.TargetPath, sourceInfo, request.Overwrite, report)
	}
	remote, err := s.openRequest(ctx, request.SourceAlias)
	if err != nil {
		return err
	}
	defer remote.Close()
	sourceInfo, err := remote.Lstat(request.SourcePath)
	if err != nil {
		return err
	}
	return s.getLocal(ctx, remote, root, request.SourcePath, relative, sourceInfo, request.Overwrite, report)
}
func (s Service) putLocal(ctx context.Context, root *os.Root, remote Remote, source, target string, info fs.FileInfo, overwrite bool, report func(int64) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if info.IsDir() {
		if targetInfo, err := remote.Lstat(target); err == nil {
			if !targetInfo.IsDir() || !overwrite {
				return ErrAlreadyExists
			}
		} else if errors.Is(err, fs.ErrNotExist) {
			if err := remote.Mkdir(target); err != nil {
				return err
			}
		} else {
			return err
		}
		dir, err := root.Open(source)
		if err != nil {
			return err
		}
		defer dir.Close()
		children, err := dir.Readdir(0)
		if err != nil {
			return err
		}
		for _, child := range children {
			childSource := path.Join(source, child.Name())
			checked, err := checkLocal(root, childSource, false)
			if err != nil {
				return err
			}
			if err := s.putLocal(ctx, root, remote, childSource, path.Join(target, child.Name()), checked, overwrite, report); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return ErrUnsupportedEntry
	}
	input, err := root.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if _, err := remote.Lstat(target); err == nil && !overwrite {
		return ErrAlreadyExists
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	temporary, err := s.temporaryPath(target)
	if err != nil {
		return err
	}
	output, err := remote.Create(temporary)
	if err != nil {
		return err
	}
	defer remote.Remove(temporary)
	written, copyErr := copyContext(ctx, &progressWriter{Writer: output, report: report}, input, info.Size())
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != info.Size() {
		return ErrConflict
	}
	after, err := checkLocal(root, source, false)
	if err != nil || metadataRevision(info) != metadataRevision(after) {
		return ErrConflict
	}
	if overwrite {
		return remote.Replace(temporary, target)
	}
	return remote.Rename(temporary, target)
}
func (s Service) getLocal(ctx context.Context, remote Remote, root *os.Root, source, target string, info fs.FileInfo, overwrite bool, report func(int64) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := checkLocal(root, target, true)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if existing != nil {
			if !existing.IsDir() || !overwrite {
				return ErrAlreadyExists
			}
		} else if err := root.Mkdir(target, info.Mode().Perm()); err != nil {
			return err
		}
		children, err := readChildren(ctx, remote, source)
		if err != nil {
			return err
		}
		for _, child := range children {
			if isInternalName(child.Name()) {
				continue
			}
			if !validLocalChildName(child.Name()) {
				return ErrInvalidPath
			}
			if err := s.getLocal(ctx, remote, root, path.Join(source, child.Name()), path.Join(target, child.Name()), child, overwrite, report); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return ErrUnsupportedEntry
	}
	if existing != nil && !overwrite {
		return ErrAlreadyExists
	}
	if existing != nil && !existing.Mode().IsRegular() {
		return ErrAlreadyExists
	}
	input, err := remote.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	suffix, err := newLocalSuffix()
	if err != nil {
		return err
	}
	temporary := path.Join(path.Dir(target), ".sshc-download-"+suffix)
	output, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	written, copyErr := copyContext(ctx, &progressWriter{Writer: output, report: report}, input, info.Size())
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != info.Size() {
		return ErrConflict
	}
	after, err := remote.Lstat(source)
	if err != nil || metadataRevision(info) != metadataRevision(after) {
		return ErrConflict
	}
	current, err := checkLocal(root, target, true)
	if err != nil {
		return err
	}
	if current != nil && (!overwrite || !current.Mode().IsRegular()) {
		return ErrAlreadyExists
	}
	if !overwrite {
		if err := root.Link(temporary, target); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return ErrAlreadyExists
			}
			return err
		}
		return nil
	}
	return root.Rename(temporary, target)
}

func newLocalSuffix() (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(suffix[:]), nil
}

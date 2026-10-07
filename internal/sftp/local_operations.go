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

// cleanLocalPath is the local counterpart of cleanPublicPath: it checks that
// value is an absolute path on the engine's file system and returns it cleaned,
// with `/` as the separator. An empty value and a leading `~` stand for the
// engine user's home. A path that cleaning would change beyond a trailing
// separator is refused rather than silently rewritten.
func cleanLocalPath(value string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if value == "" {
		value = home
	} else if belowHome, ok := cutLocalHomePrefix(value); ok {
		value = filepath.Join(home, filepath.FromSlash(belowHome))
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

// cutLocalHomePrefix reports whether value starts at the engine user's home
// and returns the part below it. Besides `~/`, the home may be followed by the
// file system's own separator, so a Windows user can type `~\Documents` as in
// PowerShell. A local path is resolved at once and never saved, so this
// spelling cannot reach a machine where `\` belongs to a file name. The saved
// start directory of a shell is different: platform.ResolveUnderHome keeps
// refusing `~\` there.
func cutLocalHomePrefix(value string) (string, bool) {
	if value == "~" {
		return "", true
	}
	if len(value) >= 2 && value[0] == '~' && os.IsPathSeparator(value[1]) {
		return value[2:], true
	}
	return "", false
}

// openLocalRoot opens the file system root that holds value and returns value
// relative to it. The folders above value are resolved first, absolute links
// included, because os.Root refuses to follow a link to an absolute path.
// value itself stays unresolved, so checkLocal still refuses a link named
// directly.
func openLocalRoot(value string) (*os.Root, string, error) {
	absolute, err := cleanLocalPath(value)
	if err != nil {
		return nil, "", err
	}
	named := filepath.FromSlash(absolute)
	parent, err := filepath.EvalSymlinks(filepath.Dir(named))
	if err != nil {
		return nil, "", err
	}
	resolved := filepath.Join(parent, filepath.Base(named))
	filesystemRoot := filepath.VolumeName(resolved) + string(filepath.Separator)
	root, err := os.OpenRoot(filesystemRoot)
	if err != nil {
		return nil, "", err
	}
	relative, err := filepath.Rel(filesystemRoot, resolved)
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

// ListLocal lists a folder of the engine's file system. Opening a folder
// follows every link on the way, as opening a remote one does. A link inside
// it is listed as a symlink with what it points to, so the pane refuses to
// hand it to a transfer, which never follows links.
func ListLocal(value string) (_ LocalListing, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	cleaned, err := cleanLocalPath(value)
	if err != nil {
		return LocalListing{}, err
	}
	directory := filepath.FromSlash(cleaned)
	info, err := os.Stat(directory)
	if err != nil {
		return LocalListing{}, err
	}
	if !info.IsDir() {
		return LocalListing{}, ErrNotDirectory
	}
	children, err := os.ReadDir(directory)
	if err != nil {
		return LocalListing{}, err
	}
	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		// Windows directory listings can cache metadata that differs from Lstat.
		// Use fresh metadata so listed revisions agree with local mutations.
		childInfo, err := os.Lstat(filepath.Join(directory, child.Name()))
		if err != nil {
			// The entry was removed after the folder was read.
			continue
		}
		entry := entryFrom(cleaned, childInfo)
		switch entry.Type {
		case EntrySymlink:
			describeLocalLink(&entry, filepath.Join(directory, child.Name()))
		case EntryFile, EntryDirectory:
		default:
			continue
		}
		entries = append(entries, entry)
	}
	sortListing(entries)
	home, err := os.UserHomeDir()
	if err != nil {
		return LocalListing{}, err
	}
	return LocalListing{Path: cleaned, Home: filepath.ToSlash(home), Entries: entries}, nil
}

// describeLocalLink fills in where a local symlink points and what it ends at,
// as describeLinks does for a remote one. The target may be an absolute path.
func describeLocalLink(entry *Entry, linkPath string) {
	if target, err := os.Readlink(linkPath); err == nil {
		entry.LinkTarget = filepath.ToSlash(target)
	}
	if info, err := os.Stat(linkPath); err == nil {
		entry.describeTarget(info)
	}
}

func (s Service) PlanLocalTransfer(ctx context.Context, request RemoteTransferRequest) (_ RemoteTransferPlan, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	var localPath, remotePath string
	if request.Operation == RemotePut {
		localPath, remotePath = request.SourcePath, request.TargetPath
	} else if request.Operation == RemoteGet {
		localPath, remotePath = request.TargetPath, request.SourcePath
	} else {
		return RemoteTransferPlan{}, ErrInvalidTransfer
	}
	if _, err := cleanLocalPath(localPath); err != nil {
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
	directory, err := root.Open(relative)
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	infos, err := directory.Readdir(0)
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
func (s Service) CopyLocal(ctx context.Context, request RemoteTransferRequest, progress func(int64) error) (err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
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
	transfer := &localCopy{
		service: s, root: root, overwrite: request.Overwrite, progress: progress,
		published: newPublishedNames(),
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
		transfer.alias, transfer.remote = request.TargetAlias, remote
		return transfer.put(ctx, relative, request.TargetPath, sourceInfo)
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
	transfer.alias, transfer.remote = request.SourceAlias, remote
	return transfer.get(ctx, request.SourcePath, relative, sourceInfo)
}

// downloadedFolderOwnerAccess is added to the mode of a folder that get
// creates. Without owner write, a remote folder such as a Go module cache
// (dr-xr-xr-x) would refuse its own children, and a rerun or a local delete
// would fail too. get does not carry remote permissions to files either: they
// are created 0600.
const downloadedFolderOwnerAccess fs.FileMode = 0o700

// localCopy runs one transfer between the engine's file system (root) and a
// remote, in either direction.
type localCopy struct {
	service Service
	root    *os.Root
	// alias names the host remote is connected to.
	alias     string
	remote    Remote
	overwrite bool
	progress  func(int64) error
	// published lets an approved overwrite replace only entries that were at
	// the target before this run.
	published *publishedNames

	transferred int64
}

func (c *localCopy) report(delta int64) error {
	c.transferred += delta
	if c.progress == nil {
		return nil
	}
	return c.progress(c.transferred)
}

func (c *localCopy) put(ctx context.Context, source, target string, info fs.FileInfo) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if info.IsDir() {
		if targetInfo, err := c.remote.Lstat(target); err == nil {
			if err := c.published.existingEntryError(target, c.overwrite); err != nil {
				return err
			}
			if !targetInfo.IsDir() {
				return differentKindError(target)
			}
		} else if errors.Is(err, fs.ErrNotExist) {
			if err := c.remote.Mkdir(target); err != nil {
				return err
			}
		} else {
			return err
		}
		c.published.record(target)
		directory, err := c.root.Open(source)
		if err != nil {
			return err
		}
		defer directory.Close()
		children, err := directory.Readdir(0)
		if err != nil {
			return err
		}
		for _, child := range children {
			childSource := path.Join(source, child.Name())
			checked, err := checkLocal(c.root, childSource, false)
			if err != nil {
				return err
			}
			if err := c.put(ctx, childSource, path.Join(target, child.Name()), checked); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return ErrUnsupportedEntry
	}
	input, err := c.root.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if _, err := c.remote.Lstat(target); err == nil {
		if err := c.published.existingEntryError(target, c.overwrite); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	temporary, err := c.service.temporaryPath(target)
	if err != nil {
		return err
	}
	output, err := c.remote.Create(temporary)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			unpublished := unpublishedFile{service: c.service, alias: c.alias, remote: c.remote, path: temporary}
			resultErr = errors.Join(resultErr, unpublished.remove(ctx))
		}
	}()
	written, copyErr := copyContext(ctx, &progressWriter{Writer: output, report: c.report}, input, info.Size())
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
	after, err := checkLocal(c.root, source, false)
	if err != nil || metadataRevision(info) != metadataRevision(after) {
		return ErrConflict
	}
	if c.overwrite {
		err = c.remote.Replace(temporary, target)
	} else {
		err = c.remote.Rename(temporary, target)
	}
	if err != nil {
		return err
	}
	published = true
	c.published.record(target)
	// The source's time is set only after publishing: until then the
	// temporary must keep the time of its last write, or a delete or move
	// of the folder would take it for an abandoned one (isAbandonedTemporary).
	return c.remote.Chtimes(target, info.ModTime())
}

func (c *localCopy) get(ctx context.Context, source, target string, info fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := checkLocal(c.root, target, true)
	if err != nil {
		return err
	}
	if existing != nil {
		if err := c.published.existingEntryError(target, c.overwrite); err != nil {
			return err
		}
	}
	if info.IsDir() {
		if existing != nil && !existing.IsDir() {
			return differentKindError(target)
		}
		if existing == nil {
			if err := c.root.Mkdir(target, info.Mode().Perm()|downloadedFolderOwnerAccess); err != nil {
				return err
			}
		}
		c.published.record(target)
		children, err := readChildren(ctx, c.remote, source)
		if err != nil {
			return err
		}
		for _, child := range children {
			if isInternalName(child.Name()) {
				continue
			}
			if !ValidLocalChildName(child.Name()) {
				return ErrInvalidPath
			}
			if err := c.get(ctx, path.Join(source, child.Name()), path.Join(target, child.Name()), child); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return ErrUnsupportedEntry
	}
	if existing != nil && !existing.Mode().IsRegular() {
		return differentKindError(target)
	}
	input, err := c.remote.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	suffix, err := newLocalSuffix()
	if err != nil {
		return err
	}
	temporary := path.Join(path.Dir(target), ".sshc-download-"+suffix)
	output, err := c.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer c.root.Remove(temporary)
	written, copyErr := copyContext(ctx, &progressWriter{Writer: output, report: c.report}, input, info.Size())
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
	after, err := c.remote.Lstat(source)
	if err != nil || metadataRevision(info) != metadataRevision(after) {
		return ErrConflict
	}
	if err := c.root.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	// The download can take long enough for the target to change meanwhile, so
	// the decision about an existing entry is made again right before publish.
	current, err := checkLocal(c.root, target, true)
	if err != nil {
		return err
	}
	if current != nil {
		if err := c.published.existingEntryError(target, c.overwrite); err != nil {
			return err
		}
		if !current.Mode().IsRegular() {
			return differentKindError(target)
		}
	}
	if !c.overwrite {
		if err := publishLocalWithoutReplace(c.root, temporary, target); err != nil {
			return err
		}
	} else if err := c.root.Rename(temporary, target); err != nil {
		return err
	}
	c.published.record(target)
	return nil
}

func newLocalSuffix() (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(suffix[:]), nil
}

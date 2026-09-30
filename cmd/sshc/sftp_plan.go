package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"

	"sshc/internal/api"
	"sshc/internal/httpserver"
	sftpcore "sshc/internal/sftp"
)

type sftpCLIRecursiveLimitError struct {
	resource string
	limit    int64
}

func (failure sftpCLIRecursiveLimitError) Error() string {
	return fmt.Sprintf("%s: maximum %s is %d", errSFTPRecursiveLimit, failure.resource, failure.limit)
}

func (sftpCLIRecursiveLimitError) Unwrap() error { return errSFTPRecursiveLimit }

type sftpCLIFile struct {
	Source       string
	Destination  string
	Size         int64
	ModifiedUnix int64
	Exists       bool
}

type sftpCLIPlan struct {
	Action      string
	Alias       string
	Source      string
	Destination string
	Directories []string
	Files       []sftpCLIFile
	Skipped     int
	// Entries left out of a recursive transfer and why: a link whose target
	// is missing, or something that is neither a file nor a directory.
	SkippedPaths []sftpCLISkip
	Bytes        int64
}

type sftpCLISkip struct {
	Path   string
	Reason string
}

func (plan *sftpCLIPlan) skip(entryPath, reason string) {
	plan.Skipped++
	plan.SkippedPaths = append(plan.SkippedPaths, sftpCLISkip{Path: entryPath, Reason: reason})
}

// sftpEntryOpensAs is what a remote entry behaves as when transferred: a
// symlink stands in for what it points to, because the engine follows links
// when it reads. An empty result is something that cannot be transferred.
func sftpEntryOpensAs(entry api.SFTPEntry) api.SFTPEntryType {
	switch entry.Type {
	case api.File, api.Directory:
		return entry.Type
	case api.Symlink:
		if entry.TargetType == nil {
			return ""
		}
		switch *entry.TargetType {
		case api.SFTPEntryTargetTypeFile:
			return api.File
		case api.SFTPEntryTargetTypeDirectory:
			return api.Directory
		}
	}
	return ""
}

// remoteListings remembers each remote directory listed while a plan is
// built, so that a directory of a thousand files is listed once rather than
// once per file.
type remoteListings struct {
	engine      *engineAPI
	alias       string
	byDirectory map[string]httpserver.SFTPListing
	failures    map[string]error
}

func newRemoteListings(engine *engineAPI, alias string) *remoteListings {
	return &remoteListings{engine: engine, alias: alias, byDirectory: map[string]httpserver.SFTPListing{}, failures: map[string]error{}}
}

func (listings *remoteListings) list(ctx context.Context, directory string) (httpserver.SFTPListing, error) {
	if listing, ok := listings.byDirectory[directory]; ok {
		return listing, nil
	}
	if err, ok := listings.failures[directory]; ok {
		return httpserver.SFTPListing{}, err
	}
	// Nothing exists below a directory that does not exist; a put into a new
	// tree would otherwise ask about every directory it is about to create.
	if parent := path.Dir(directory); parent != directory {
		if err, ok := listings.failures[parent]; ok && sftpIsNotFound(err) {
			listings.failures[directory] = err
			return httpserver.SFTPListing{}, err
		}
	}
	listing, err := sftpList(ctx, listings.engine, listings.alias, directory)
	if err != nil {
		listings.failures[directory] = err
		return httpserver.SFTPListing{}, err
	}
	listings.byDirectory[directory] = listing
	return listing, nil
}

func (listings *remoteListings) stat(ctx context.Context, remotePath string) (api.SFTPEntry, error) {
	cleaned := path.Clean(remotePath)
	if cleaned == "/" {
		return api.SFTPEntry{Name: "/", Path: "/", Type: api.Directory}, nil
	}
	listing, err := listings.list(ctx, path.Dir(cleaned))
	if err != nil {
		return api.SFTPEntry{}, err
	}
	for _, entry := range listing.Entries {
		if entry.Path == cleaned {
			return entry, nil
		}
	}
	return api.SFTPEntry{}, engineProblem{Status: http.StatusNotFound, Code: "sftp_not_found"}
}

type sftpCLIRecursiveBudget struct {
	maxDepth   int
	maxEntries int
	maxBytes   int64
	entries    int
	bytes      int64
}

func recursiveSFTPCLIBudget(called sftpInvocation) sftpCLIRecursiveBudget {
	depth := called.MaxDepth
	if depth <= 0 || depth > sftpCLIMaxRecursiveDepth {
		depth = sftpCLIDefaultRecursiveDepth
	}
	entries := called.MaxEntries
	if entries <= 0 || entries > sftpCLIMaxRecursiveEntries {
		entries = sftpCLIDefaultRecursiveEntries
	}
	totalMiB := called.MaxTotalMiB
	if totalMiB <= 0 || totalMiB > sftpCLIMaxRecursiveMiB {
		totalMiB = sftpCLIDefaultRecursiveBytes >> 20
	}
	bytes := totalMiB << 20
	return sftpCLIRecursiveBudget{
		maxDepth: depth, maxEntries: entries, maxBytes: bytes,
		// The selected root is also materialized as a directory.
		entries: 1,
	}
}

func (budget *sftpCLIRecursiveBudget) include(entry api.SFTPEntry, depth int) error {
	if depth > budget.maxDepth {
		return sftpCLIRecursiveLimitError{resource: "depth", limit: int64(budget.maxDepth)}
	}
	if budget.entries >= budget.maxEntries {
		return sftpCLIRecursiveLimitError{resource: "entries", limit: int64(budget.maxEntries)}
	}
	budget.entries++
	if sftpEntryOpensAs(entry) != api.File {
		return nil
	}
	if entry.Size < 0 {
		return errEngineInvalidResponse
	}
	if budget.bytes > budget.maxBytes || entry.Size > budget.maxBytes-budget.bytes {
		return sftpCLIRecursiveLimitError{resource: "total size in bytes", limit: budget.maxBytes}
	}
	budget.bytes += entry.Size
	return nil
}

func buildSFTPGetPlan(ctx context.Context, engine *engineAPI, called sftpInvocation) (sftpCLIPlan, error) {
	if !path.IsAbs(called.Source) {
		return sftpCLIPlan{}, errSFTPRemotePath
	}
	source, err := sftpRemoteStat(ctx, engine, called.Alias, path.Clean(called.Source))
	if err != nil {
		return sftpCLIPlan{}, err
	}
	destination, err := filepath.Abs(called.Destination)
	if err != nil {
		return sftpCLIPlan{}, err
	}
	plan := sftpCLIPlan{Action: "get", Alias: called.Alias, Source: source.Path, Destination: destination}
	info, localErr := os.Lstat(destination)
	localExists := localErr == nil
	if localErr != nil && !errors.Is(localErr, fs.ErrNotExist) {
		return plan, localErr
	}
	if sftpEntryOpensAs(source) == api.File {
		if localExists && info.IsDir() {
			destination = filepath.Join(destination, source.Name)
		}
		exists, err := localFileConflict(destination)
		if err != nil {
			return plan, err
		}
		plan.Destination = destination
		plan.Files = []sftpCLIFile{{Source: source.Path, Destination: destination, Size: source.Size, ModifiedUnix: modifiedUnix(source), Exists: exists}}
		plan.Bytes = source.Size
		return plan, nil
	}
	if sftpEntryOpensAs(source) != api.Directory {
		return plan, fmt.Errorf("%w: %s", errSFTPUnsupportedLocal, source.Path)
	}
	if !called.Recursive {
		return plan, errSFTPRecursiveRequired
	}
	root := destination
	if localExists {
		if !info.IsDir() {
			return plan, errSFTPTypeMismatch
		}
		root = filepath.Join(destination, source.Name)
	}
	plan.Destination = root
	plan.Directories = append(plan.Directories, root)
	budget := recursiveSFTPCLIBudget(called)
	listings := newRemoteListings(engine, called.Alias)
	if err := walkRemoteGetPlan(ctx, listings, source.Path, root, 0, &budget, &plan); err != nil {
		return plan, err
	}
	return plan, nil
}

// modifiedUnix is the entry's modification time in milliseconds, or 0 when
// the engine gave none, so the local copy can be given the same time.
func modifiedUnix(entry api.SFTPEntry) int64 {
	if entry.ModifiedAt.IsZero() {
		return 0
	}
	return entry.ModifiedAt.UnixMilli()
}

func walkRemoteGetPlan(
	ctx context.Context,
	listings *remoteListings,
	remoteRoot, localRoot string,
	depth int,
	budget *sftpCLIRecursiveBudget,
	plan *sftpCLIPlan,
) error {
	listing, err := listings.list(ctx, remoteRoot)
	if err != nil {
		return err
	}
	sort.Slice(listing.Entries, func(i, j int) bool { return listing.Entries[i].Name < listing.Entries[j].Name })
	for _, entry := range listing.Entries {
		if !sftpcore.ValidLocalChildName(entry.Name) {
			return fmt.Errorf("%w: %q", errSFTPLocalName, entry.Name)
		}
		if entry.Path != path.Join(remoteRoot, entry.Name) {
			return errEngineInvalidResponse
		}
		target := filepath.Join(localRoot, entry.Name)
		switch sftpEntryOpensAs(entry) {
		case api.Directory:
			if err := budget.include(entry, depth+1); err != nil {
				return err
			}
			if info, err := os.Lstat(target); err == nil && !info.IsDir() {
				return errSFTPTypeMismatch
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			plan.Directories = append(plan.Directories, target)
			if err := walkRemoteGetPlan(ctx, listings, entry.Path, target, depth+1, budget, plan); err != nil {
				return err
			}
		case api.File:
			if err := budget.include(entry, depth+1); err != nil {
				return err
			}
			exists, err := localFileConflict(target)
			if err != nil {
				return err
			}
			plan.Files = append(plan.Files, sftpCLIFile{Source: entry.Path, Destination: target, Size: entry.Size, ModifiedUnix: modifiedUnix(entry), Exists: exists})
			plan.Bytes += entry.Size
		default:
			switch entry.Type {
			case api.Symlink:
				plan.skip(entry.Path, "the link target cannot be read")
			case api.Other:
				plan.skip(entry.Path, "not a file or a directory")
			default:
				return errEngineInvalidResponse
			}
		}
	}
	return nil
}

func buildSFTPPutPlan(ctx context.Context, engine *engineAPI, called sftpInvocation) (sftpCLIPlan, error) {
	if !path.IsAbs(called.Destination) {
		return sftpCLIPlan{}, errSFTPRemotePath
	}
	source, err := filepath.Abs(called.Source)
	if err != nil {
		return sftpCLIPlan{}, err
	}
	// A symlink given on the command line is followed, as WinSCP and scp do.
	info, err := os.Stat(source)
	if err != nil {
		return sftpCLIPlan{}, markLocalSourceMissing(err)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return sftpCLIPlan{}, fmt.Errorf("%w: %s", errSFTPUnsupportedLocal, source)
	}
	destination := path.Clean(called.Destination)
	listings := newRemoteListings(engine, called.Alias)
	remoteDestination, remoteErr := listings.stat(ctx, destination)
	remoteExists := remoteErr == nil
	if remoteErr != nil && !sftpIsNotFound(remoteErr) {
		return sftpCLIPlan{}, remoteErr
	}
	plan := sftpCLIPlan{Action: "put", Alias: called.Alias, Source: source, Destination: destination}
	if info.Mode().IsRegular() {
		if remoteExists && sftpEntryOpensAs(remoteDestination) == api.Directory {
			destination = path.Join(destination, filepath.Base(source))
			remoteDestination, remoteErr = listings.stat(ctx, destination)
			remoteExists = remoteErr == nil
			if remoteErr != nil && !sftpIsNotFound(remoteErr) {
				return plan, remoteErr
			}
		}
		if remoteExists && sftpEntryOpensAs(remoteDestination) != api.File {
			return plan, errSFTPTypeMismatch
		}
		if !remoteExists {
			if _, err := listings.stat(ctx, path.Dir(destination)); sftpIsNotFound(err) {
				return plan, fmt.Errorf("%w: %s", errSFTPRemoteDirectoryMissing, path.Dir(destination))
			} else if err != nil {
				return plan, err
			}
		}
		plan.Destination = destination
		plan.Files = []sftpCLIFile{{Source: source, Destination: destination, Size: info.Size(), ModifiedUnix: info.ModTime().UnixMilli(), Exists: remoteExists}}
		plan.Bytes = info.Size()
		return plan, nil
	}
	if !called.Recursive {
		return plan, errSFTPRecursiveRequired
	}
	root := destination
	if remoteExists {
		if sftpEntryOpensAs(remoteDestination) != api.Directory {
			return plan, errSFTPTypeMismatch
		}
		root = path.Join(destination, filepath.Base(source))
	}
	plan.Destination = root
	plan.Directories = append(plan.Directories, root)
	walk := localPutWalk{ctx: ctx, listings: listings, plan: &plan, ancestors: map[string]bool{}}
	if err := walk.directory(source, root, 0); err != nil {
		return plan, err
	}
	return plan, nil
}

// localPutWalk collects a local tree for upload. Symlinks are followed, as
// WinSCP and scp do, so a linked directory is uploaded under the link's name
// too; only a link back into a directory being walked is skipped, because it
// would never end. A link whose target is missing is skipped rather than
// failing the tree.
type localPutWalk struct {
	ctx      context.Context
	listings *remoteListings
	plan     *sftpCLIPlan
	// The resolved paths of the directories above the one being walked.
	ancestors map[string]bool
}

func (walk *localPutWalk) directory(localDirectory, remoteDirectory string, depth int) error {
	if depth > sftpCLIMaxRecursiveDepth {
		return sftpCLIRecursiveLimitError{resource: "depth", limit: int64(sftpCLIMaxRecursiveDepth)}
	}
	resolved, err := filepath.EvalSymlinks(localDirectory)
	if err != nil {
		return markLocalSourceMissing(err)
	}
	if walk.ancestors[resolved] {
		walk.plan.skip(localDirectory, "a link back into a directory being uploaded")
		return nil
	}
	walk.ancestors[resolved] = true
	defer delete(walk.ancestors, resolved)
	entries, err := os.ReadDir(localDirectory)
	if err != nil {
		return markLocalSourceMissing(err)
	}
	for _, entry := range entries {
		localPath := filepath.Join(localDirectory, entry.Name())
		remotePath := path.Join(remoteDirectory, entry.Name())
		info, err := os.Stat(localPath)
		if err != nil {
			if entry.Type()&os.ModeSymlink != 0 && errors.Is(err, fs.ErrNotExist) {
				walk.plan.skip(localPath, "the link target does not exist")
				continue
			}
			return markLocalSourceMissing(err)
		}
		remoteEntry, statErr := walk.listings.stat(walk.ctx, remotePath)
		exists := statErr == nil
		if statErr != nil && !sftpIsNotFound(statErr) {
			return statErr
		}
		switch {
		case info.IsDir():
			if exists && sftpEntryOpensAs(remoteEntry) != api.Directory {
				return errSFTPTypeMismatch
			}
			walk.plan.Directories = append(walk.plan.Directories, remotePath)
			if err := walk.directory(localPath, remotePath, depth+1); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if exists && sftpEntryOpensAs(remoteEntry) != api.File {
				return errSFTPTypeMismatch
			}
			walk.plan.Files = append(walk.plan.Files, sftpCLIFile{Source: localPath, Destination: remotePath, Size: info.Size(), ModifiedUnix: info.ModTime().UnixMilli(), Exists: exists})
			walk.plan.Bytes += info.Size()
		default:
			walk.plan.skip(localPath, "not a file or a directory")
		}
	}
	return nil
}

func transferableSFTPFiles(files []sftpCLIFile, skipExisting bool) []sftpCLIFile {
	if !skipExisting {
		return files
	}
	selected := make([]sftpCLIFile, 0, len(files))
	for _, file := range files {
		if !file.Exists {
			selected = append(selected, file)
		}
	}
	return selected
}

func sftpList(ctx context.Context, engine *engineAPI, alias, remotePath string) (httpserver.SFTPListing, error) {
	var listing httpserver.SFTPListing
	requestPath := "/api/v1/sftp/" + url.PathEscape(alias) + "/entries?" + url.Values{"path": {remotePath}}.Encode()
	err := engine.getJSON(ctx, requestPath, &listing)
	return listing, err
}

func sftpRemoteStat(ctx context.Context, engine *engineAPI, alias, remotePath string) (api.SFTPEntry, error) {
	cleaned := path.Clean(remotePath)
	if cleaned == "/" {
		return api.SFTPEntry{Name: "/", Path: "/", Type: api.Directory}, nil
	}
	listing, err := sftpList(ctx, engine, alias, path.Dir(cleaned))
	if err != nil {
		return api.SFTPEntry{}, err
	}
	for _, entry := range listing.Entries {
		if entry.Path == cleaned {
			return entry, nil
		}
	}
	return api.SFTPEntry{}, engineProblem{Status: http.StatusNotFound, Code: "sftp_not_found"}
}

func sftpEnsureRemoteDirectory(ctx context.Context, engine *engineAPI, alias, remotePath string) error {
	if remotePath == "/" {
		return nil
	}
	entry, err := sftpRemoteStat(ctx, engine, alias, remotePath)
	if err == nil {
		if entry.Type != api.Directory {
			return errSFTPTypeMismatch
		}
		return nil
	}
	if !sftpIsNotFound(err) {
		return err
	}
	if err := sftpEnsureRemoteDirectory(ctx, engine, alias, path.Dir(remotePath)); err != nil {
		return err
	}
	var created api.SFTPEntry
	return engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/"+url.PathEscape(alias)+"/entries", map[string]string{"path": remotePath, "type": "directory"}, &created)
}

// markLocalSourceMissing tags a vanished local source with errSFTPLocalMissing;
// other errors pass through unchanged.
func markLocalSourceMissing(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %w", errSFTPLocalMissing, err)
	}
	return err
}

func localFileConflict(localPath string) (bool, error) {
	info, err := os.Lstat(localPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errSFTPTypeMismatch
	}
	return true, nil
}

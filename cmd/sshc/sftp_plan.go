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
	"strings"
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
	Bytes       int64
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

func (budget *sftpCLIRecursiveBudget) include(entry sftpCLIEntry, depth int) error {
	if depth > budget.maxDepth {
		return sftpCLIRecursiveLimitError{resource: "depth", limit: int64(budget.maxDepth)}
	}
	if budget.entries >= budget.maxEntries {
		return sftpCLIRecursiveLimitError{resource: "entries", limit: int64(budget.maxEntries)}
	}
	budget.entries++
	if entry.Type != "file" {
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
	if source.Type == "file" {
		if localExists && info.IsDir() {
			destination = filepath.Join(destination, source.Name)
		}
		exists, err := localFileConflict(destination)
		if err != nil {
			return plan, err
		}
		plan.Destination = destination
		plan.Files = []sftpCLIFile{{Source: source.Path, Destination: destination, Size: source.Size, Exists: exists}}
		plan.Bytes = source.Size
		return plan, nil
	}
	if source.Type != "directory" {
		return plan, errSFTPUnsupportedLocal
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
	if err := walkRemoteGetPlan(ctx, engine, called.Alias, source.Path, root, 0, &budget, &plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func walkRemoteGetPlan(
	ctx context.Context,
	engine *engineAPI,
	alias, remoteRoot, localRoot string,
	depth int,
	budget *sftpCLIRecursiveBudget,
	plan *sftpCLIPlan,
) error {
	listing, err := sftpList(ctx, engine, alias, remoteRoot)
	if err != nil {
		return err
	}
	sort.Slice(listing.Entries, func(i, j int) bool { return listing.Entries[i].Name < listing.Entries[j].Name })
	for _, entry := range listing.Entries {
		if entry.Name == "" || entry.Name == "." || entry.Name == ".." || path.Base(entry.Name) != entry.Name || strings.Contains(entry.Name, "\\") {
			return errEngineInvalidResponse
		}
		if entry.Path != path.Join(remoteRoot, entry.Name) {
			return errEngineInvalidResponse
		}
		if err := budget.include(entry, depth+1); err != nil {
			return err
		}
		target := filepath.Join(localRoot, entry.Name)
		switch entry.Type {
		case "directory":
			if info, err := os.Lstat(target); err == nil && !info.IsDir() {
				return errSFTPTypeMismatch
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			plan.Directories = append(plan.Directories, target)
			if err := walkRemoteGetPlan(ctx, engine, alias, entry.Path, target, depth+1, budget, plan); err != nil {
				return err
			}
		case "file":
			exists, err := localFileConflict(target)
			if err != nil {
				return err
			}
			plan.Files = append(plan.Files, sftpCLIFile{Source: entry.Path, Destination: target, Size: entry.Size, Exists: exists})
			plan.Bytes += entry.Size
		case "symlink", "other":
			return fmt.Errorf("%w: %s", errSFTPUnsupportedLocal, entry.Path)
		default:
			return errEngineInvalidResponse
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
	info, err := os.Lstat(source)
	if err != nil {
		return sftpCLIPlan{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return sftpCLIPlan{}, errSFTPUnsupportedLocal
	}
	destination := path.Clean(called.Destination)
	remoteDestination, remoteErr := sftpRemoteStat(ctx, engine, called.Alias, destination)
	remoteExists := remoteErr == nil
	if remoteErr != nil && !sftpIsNotFound(remoteErr) {
		return sftpCLIPlan{}, remoteErr
	}
	plan := sftpCLIPlan{Action: "put", Alias: called.Alias, Source: source, Destination: destination}
	if info.Mode().IsRegular() {
		if remoteExists && remoteDestination.Type == "directory" {
			destination = path.Join(destination, filepath.Base(source))
			remoteDestination, remoteErr = sftpRemoteStat(ctx, engine, called.Alias, destination)
			remoteExists = remoteErr == nil
			if remoteErr != nil && !sftpIsNotFound(remoteErr) {
				return plan, remoteErr
			}
		}
		if remoteExists && remoteDestination.Type != "file" {
			return plan, errSFTPTypeMismatch
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
		if remoteDestination.Type != "directory" {
			return plan, errSFTPTypeMismatch
		}
		root = path.Join(destination, filepath.Base(source))
	}
	plan.Destination = root
	plan.Directories = append(plan.Directories, root)
	err = filepath.WalkDir(source, func(localPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if localPath == source {
			return nil
		}
		relative, err := filepath.Rel(source, localPath)
		if err != nil {
			return err
		}
		remotePath := path.Join(root, filepath.ToSlash(relative))
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errSFTPUnsupportedLocal, localPath)
		}
		remoteEntry, statErr := sftpRemoteStat(ctx, engine, called.Alias, remotePath)
		exists := statErr == nil
		if statErr != nil && !sftpIsNotFound(statErr) {
			return statErr
		}
		if entry.IsDir() {
			if exists && remoteEntry.Type != "directory" {
				return errSFTPTypeMismatch
			}
			plan.Directories = append(plan.Directories, remotePath)
			return nil
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", errSFTPUnsupportedLocal, localPath)
		}
		if exists && remoteEntry.Type != "file" {
			return errSFTPTypeMismatch
		}
		plan.Files = append(plan.Files, sftpCLIFile{Source: localPath, Destination: remotePath, Size: entryInfo.Size(), ModifiedUnix: entryInfo.ModTime().UnixMilli(), Exists: exists})
		plan.Bytes += entryInfo.Size()
		return nil
	})
	return plan, err
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

func sftpList(ctx context.Context, engine *engineAPI, alias, remotePath string) (sftpCLIListing, error) {
	var listing sftpCLIListing
	requestPath := "/api/v1/sftp/" + url.PathEscape(alias) + "/entries?" + url.Values{"path": {remotePath}}.Encode()
	err := engine.getJSON(ctx, requestPath, &listing)
	return listing, err
}

func sftpRemoteStat(ctx context.Context, engine *engineAPI, alias, remotePath string) (sftpCLIEntry, error) {
	cleaned := path.Clean(remotePath)
	if cleaned == "/" {
		return sftpCLIEntry{Name: "/", Path: "/", Type: "directory"}, nil
	}
	listing, err := sftpList(ctx, engine, alias, path.Dir(cleaned))
	if err != nil {
		return sftpCLIEntry{}, err
	}
	for _, entry := range listing.Entries {
		if entry.Path == cleaned {
			return entry, nil
		}
	}
	return sftpCLIEntry{}, engineProblem{Status: http.StatusNotFound, Code: "sftp_not_found"}
}

func sftpEnsureRemoteDirectory(ctx context.Context, engine *engineAPI, alias, remotePath string) error {
	if remotePath == "/" {
		return nil
	}
	entry, err := sftpRemoteStat(ctx, engine, alias, remotePath)
	if err == nil {
		if entry.Type != "directory" {
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
	var created sftpCLIEntry
	return engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/"+url.PathEscape(alias)+"/entries", map[string]string{"path": remotePath, "type": "directory"}, &created)
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

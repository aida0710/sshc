package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	sftpcore "sshc/internal/sftp"
)

const (
	sftpCLIChunkBytes = 1 << 20

	// Recursive get first builds a complete conflict-safe plan. Keep that walk
	// within the same default tree budget as the Web folder archive so a hostile
	// or accidentally broad remote path cannot grow memory, requests, or the
	// eventual download without bound. Callers can deliberately raise each
	// limit, but even overrides remain finite.
	sftpCLIDefaultRecursiveDepth   = 64
	sftpCLIDefaultRecursiveEntries = 10_000
	sftpCLIDefaultRecursiveBytes   = int64(1 << 30)
	sftpCLIMaxRecursiveDepth       = 256
	sftpCLIMaxRecursiveEntries     = 1_000_000
	sftpCLIMaxRecursiveMiB         = int64(8 * 1024 * 1024)
)

var (
	errSFTPRecursiveRequired = errors.New("recursive transfer requires --recursive")
	errSFTPExisting          = errors.New("destination already exists")
	errSFTPTypeMismatch      = errors.New("source and destination types do not match")
	errSFTPUnsupportedLocal  = errors.New("local source contains an unsupported file type")
	errSFTPMissingRevision   = errors.New("download response has no revision")
	errSFTPRemotePath        = errors.New("remote path must be absolute")
	errSFTPRecursiveLimit    = errors.New("recursive download safety limit exceeded")
)

type sftpCLIRecursiveLimitError struct {
	resource string
	limit    int64
}

func (failure sftpCLIRecursiveLimitError) Error() string {
	return fmt.Sprintf("%s: maximum %s is %d", errSFTPRecursiveLimit, failure.resource, failure.limit)
}

func (sftpCLIRecursiveLimitError) Unwrap() error { return errSFTPRecursiveLimit }

type sftpCLIEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModifiedAt string `json:"modifiedAt"`
	Revision   string `json:"revision"`
}

type sftpCLIListing struct {
	Path    string         `json:"path"`
	Entries []sftpCLIEntry `json:"entries"`
}

type sftpCLIUpload struct {
	ID               string               `json:"id"`
	Path             string               `json:"path"`
	Offset           int64                `json:"offset"`
	Size             int64                `json:"size"`
	ExpectedRevision string               `json:"expectedRevision"`
	CompletedRanges  []sftpCLIUploadRange `json:"completedRanges"`
	Parallelism      int                  `json:"parallelism"`
	ChunkBytes       int64                `json:"chunkBytes"`
}

type sftpCLIUploadRange struct {
	Offset int64 `json:"offset"`
	Size   int64 `json:"size"`
}

type sftpCLITransfer struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Revision string `json:"revision"`
}

type sftpCLIResult struct {
	Action      string `json:"action"`
	Alias       string `json:"alias"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Bytes       int64  `json:"bytes"`
	Skipped     int    `json:"skipped"`
	Overwritten int    `json:"overwritten"`
	DryRun      bool   `json:"dryRun"`
}

type sftpCLITransferQueue struct {
	MaxConcurrent              int               `json:"maxConcurrent"`
	ClearCompletedAfterSeconds int               `json:"clearCompletedAfterSeconds"`
	ProcessingStopped          bool              `json:"processingStopped"`
	LargeFileThresholdBytes    int64             `json:"largeFileThresholdBytes"`
	LargeFileParallelism       int               `json:"largeFileParallelism"`
	LargeFileChunkBytes        int64             `json:"largeFileChunkBytes"`
	Jobs                       []json.RawMessage `json:"jobs"`
}

type sftpCLIDownloadPart struct {
	Index            int   `json:"index"`
	TransferredBytes int64 `json:"transferredBytes"`
	TotalBytes       int64 `json:"totalBytes"`
}

type sftpCLIProgressJob struct {
	ID            string                `json:"id"`
	DownloadParts []sftpCLIDownloadPart `json:"downloadParts"`
}

type sftpCLIProgressDisplay struct {
	engine    *engineAPI
	output    io.Writer
	refreshMu sync.Mutex
	mu        sync.Mutex
	tracked   map[string]string
	jobs      map[string]sftpCLIProgressJob
	lines     int
}

type sftpCLISettingsResult struct {
	SplitSizeMiB int64 `json:"splitSizeMiB"`
	SplitJobs    int   `json:"splitJobs"`
	ChunkSizeMiB int64 `json:"chunkSizeMiB"`
}

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

func runSFTP(
	ctx context.Context,
	called sftpInvocation,
	stateDir string,
	client *http.Client,
	stdout, stderr io.Writer,
	confirm actionConfirmer,
) int {
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishSFTPFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()
	if called.Action == sftpSettings {
		return runSFTPSettings(ctx, engine, called, stdout, stderr)
	}

	var plan sftpCLIPlan
	if called.Action == sftpGet {
		plan, err = buildSFTPGetPlan(ctx, engine, called)
	} else {
		plan, err = buildSFTPPutPlan(ctx, engine, called)
	}
	if err != nil {
		return finishSFTPFailure(called.JSON, err, stdout, stderr)
	}
	conflicts := 0
	for _, file := range plan.Files {
		if file.Exists {
			conflicts++
		}
	}
	if conflicts > 0 && !called.SkipExisting && !called.Overwrite {
		return finishSFTPFailure(called.JSON, fmt.Errorf("%w: %d file(s); use --overwrite or --skip-existing", errSFTPExisting, conflicts), stdout, stderr)
	}
	if called.Overwrite && conflicts > 0 && !called.DryRun {
		fmt.Fprintf(stderr, "sshc: %d existing destination file(s) will be replaced\n", conflicts)
		confirmed, code := confirmAction(ctx, called.Yes, "Continue? [y/N] ", confirm, stderr)
		if code != 0 {
			if called.JSON {
				failure := commandFailure{Kind: "confirmation_required", Retryable: false}
				if errors.Is(ctx.Err(), context.Canceled) {
					failure = commandFailure{Kind: "canceled", Retryable: true}
				}
				_ = writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: false, Failure: &failure})
			}
			return code
		}
		if !confirmed {
			fmt.Fprintln(stderr, "sshc: canceled")
			if called.JSON {
				failure := commandFailure{Kind: "confirmation_declined", Retryable: false}
				_ = writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: false, Failure: &failure})
			}
			return 0
		}
	}

	result := sftpCLIResult{
		Action: plan.Action, Alias: plan.Alias, Source: plan.Source, Destination: plan.Destination,
		Directories: len(plan.Directories), Skipped: plan.Skipped, DryRun: called.DryRun,
	}
	for _, file := range plan.Files {
		if file.Exists && called.SkipExisting {
			result.Skipped++
			continue
		}
		result.Files++
		result.Bytes += file.Size
		if file.Exists {
			result.Overwritten++
		}
	}
	if called.DryRun {
		return finishSFTPSuccess(called.JSON, result, stdout)
	}

	if called.Action == sftpGet {
		err = executeSFTPGet(ctx, engine, plan, called, stderr)
	} else {
		err = executeSFTPPut(ctx, engine, plan, called, stderr)
	}
	if err != nil {
		return finishSFTPFailure(called.JSON, err, stdout, stderr)
	}
	return finishSFTPSuccess(called.JSON, result, stdout)
}

func runSFTPSettings(ctx context.Context, engine *engineAPI, called sftpInvocation, stdout, stderr io.Writer) int {
	var settings sftpCLITransferQueue
	if err := engine.sendJSON(ctx, http.MethodGet, "/api/v1/sftp/transfers", nil, &settings); err != nil {
		return finishSFTPFailure(called.JSON, err, stdout, stderr)
	}
	if !validSFTPCLITransferSettings(settings) {
		return finishSFTPFailure(called.JSON, errEngineInvalidResponse, stdout, stderr)
	}
	if called.SplitSizeMiB > 0 {
		settings.LargeFileThresholdBytes = int64(called.SplitSizeMiB) << 20
	}
	if called.SplitJobs > 0 {
		settings.LargeFileParallelism = called.SplitJobs
	}
	if called.ChunkSizeMiB > 0 {
		settings.LargeFileChunkBytes = int64(called.ChunkSizeMiB) << 20
	}
	if called.SplitSizeMiB > 0 || called.SplitJobs > 0 || called.ChunkSizeMiB > 0 {
		request := map[string]any{
			"maxConcurrent": settings.MaxConcurrent, "clearCompletedAfterSeconds": settings.ClearCompletedAfterSeconds,
			"processingStopped": settings.ProcessingStopped, "largeFileThresholdBytes": settings.LargeFileThresholdBytes,
			"largeFileParallelism": settings.LargeFileParallelism, "largeFileChunkBytes": settings.LargeFileChunkBytes,
		}
		if err := engine.sendJSON(ctx, http.MethodPut, "/api/v1/sftp/transfers/settings", request, &settings); err != nil {
			return finishSFTPFailure(called.JSON, err, stdout, stderr)
		}
		if !validSFTPCLITransferSettings(settings) {
			return finishSFTPFailure(called.JSON, errEngineInvalidResponse, stdout, stderr)
		}
	}
	result := sftpCLISettingsResult{
		SplitSizeMiB: settings.LargeFileThresholdBytes >> 20,
		SplitJobs:    settings.LargeFileParallelism,
		ChunkSizeMiB: settings.LargeFileChunkBytes >> 20,
	}
	if called.JSON {
		if err := writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: true, Result: result}); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "split-size  %d MiB\nsplit-jobs  %d\nchunk-size  %d MiB\n", result.SplitSizeMiB, result.SplitJobs, result.ChunkSizeMiB)
	return 0
}

func validSFTPCLITransferSettings(settings sftpCLITransferQueue) bool {
	return settings.MaxConcurrent >= 1 && settings.MaxConcurrent <= 8 &&
		settings.ClearCompletedAfterSeconds >= 0 && settings.ClearCompletedAfterSeconds <= 86400 &&
		settings.LargeFileThresholdBytes >= 16<<20 && settings.LargeFileThresholdBytes <= 1024<<20 &&
		settings.LargeFileParallelism >= 1 && settings.LargeFileParallelism <= sftpcore.MaxLargeFileParallelism &&
		settings.LargeFileChunkBytes >= 8<<20 && settings.LargeFileChunkBytes <= int64(4096)<<20
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

func executeSFTPGet(ctx context.Context, engine *engineAPI, plan sftpCLIPlan, called sftpInvocation, stderr io.Writer) error {
	for _, directory := range plan.Directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	batchID, err := sftpIdentifier("batch")
	if err != nil {
		return err
	}
	files := transferableSFTPFiles(plan.Files, called.SkipExisting)
	for _, file := range files {
		fmt.Fprintf(stderr, "get  %s:%s -> %s\n", plan.Alias, file.Source, file.Destination)
	}
	progress := newSFTPCLIProgressDisplay(engine, stderr, called.JSON)
	return runSFTPFileWorkers(ctx, files, called.Jobs, func(workerContext context.Context, file sftpCLIFile) error {
		return sftpDownloadFile(workerContext, engine, plan.Alias, batchID, file, called.SplitSizeMiB, called.SplitJobs, called.ChunkSizeMiB, progress)
	})
}

func executeSFTPPut(ctx context.Context, engine *engineAPI, plan sftpCLIPlan, called sftpInvocation, stderr io.Writer) error {
	for _, directory := range plan.Directories {
		if err := sftpEnsureRemoteDirectory(ctx, engine, plan.Alias, directory); err != nil {
			return err
		}
	}
	batchID, err := sftpIdentifier("batch")
	if err != nil {
		return err
	}
	files := transferableSFTPFiles(plan.Files, called.SkipExisting)
	for _, file := range files {
		fmt.Fprintf(stderr, "put  %s -> %s:%s\n", file.Source, plan.Alias, file.Destination)
	}
	return runSFTPFileWorkers(ctx, files, called.Jobs, func(workerContext context.Context, file sftpCLIFile) error {
		return sftpUploadFile(workerContext, engine, plan.Alias, batchID, file, called.Overwrite, called.SplitSizeMiB, called.SplitJobs, called.ChunkSizeMiB)
	})
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

// runSFTPFileWorkers bounds one CLI invocation independently from the
// engine-wide queue limit. The engine remains authoritative when browsers or
// another CLI are transferring at the same time.
func runSFTPFileWorkers(
	ctx context.Context, files []sftpCLIFile, jobs int,
	transfer func(context.Context, sftpCLIFile) error,
) error {
	if len(files) == 0 {
		return nil
	}
	if jobs < 1 {
		jobs = 1
	}
	workerContext, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	work := make(chan sftpCLIFile)
	var workers sync.WaitGroup
	for range min(jobs, len(files)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for file := range work {
				if err := transfer(workerContext, file); err != nil {
					cancel(err)
					return
				}
			}
		}()
	}
	for _, file := range files {
		select {
		case work <- file:
		case <-workerContext.Done():
			close(work)
			workers.Wait()
			return context.Cause(workerContext)
		}
	}
	close(work)
	workers.Wait()
	return context.Cause(workerContext)
}

func sftpDownloadFile(
	ctx context.Context, engine *engineAPI, alias, batchID string, file sftpCLIFile, splitSizeMiB, splitJobs, chunkSizeMiB int,
	progress *sftpCLIProgressDisplay,
) (returnErr error) {
	jobID, err := sftpIdentifier("get")
	if err != nil {
		return err
	}
	if err := sftpCreateDownloadJob(ctx, engine, jobID, batchID, alias, file, splitSizeMiB, splitJobs, chunkSizeMiB); err != nil {
		return err
	}
	if progress != nil {
		progress.track(jobID, path.Base(file.Source))
	}
	defer func() {
		if returnErr != nil {
			action := "fail"
			if errors.Is(returnErr, context.Canceled) {
				action = "cancel"
			}
			_ = sftpJobAction(context.Background(), engine, jobID, action)
		}
	}()
	if err := sftpStartJob(ctx, engine, jobID); err != nil {
		return err
	}
	requestPath := "/api/v1/sftp/" + url.PathEscape(alias) + "/download?" + url.Values{"path": {file.Source}, "jobId": {jobID}}.Encode()
	response, err := waitForSFTPDownloadResponse(ctx, engine, requestPath, progress)
	if err != nil {
		return err
	}
	revision := response.Header.Get("ETag")
	if revision == "" {
		response.Body.Close()
		return errSFTPMissingRevision
	}
	if err := os.MkdirAll(filepath.Dir(file.Destination), 0o700); err != nil {
		response.Body.Close()
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(file.Destination), ".sshc-sftp-*")
	if err != nil {
		response.Body.Close()
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	download := io.Reader(response.Body)
	if file.Size >= 0 {
		// The listing used to approve the recursive plan fixes this file's
		// contribution to the total-size budget. Read one sentinel byte past
		// that size to detect a replacement without filling local storage with
		// an unexpectedly grown remote file.
		limit := file.Size
		if limit < int64(^uint64(0)>>1) {
			limit++
		}
		download = io.LimitReader(response.Body, limit)
	}
	written, copyErr := io.Copy(temporary, download)
	closeResponseErr := response.Body.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeResponseErr != nil {
		return closeResponseErr
	}
	if file.Size >= 0 && written != file.Size {
		return io.ErrUnexpectedEOF
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if !file.Exists {
		if _, err := os.Lstat(file.Destination); err == nil {
			return errSFTPExisting
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := publishSFTPDownload(temporaryName, file.Destination); err != nil {
		return err
	}
	temporaryName = ""
	checkpoint := map[string]any{"offset": written, "revision": revision}
	var ignored map[string]any
	if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/transfers/"+url.PathEscape(jobID)+"/download-checkpoint", checkpoint, &ignored); err != nil {
		return err
	}
	return sftpJobAction(ctx, engine, jobID, "complete")
}

type sftpRawResponse struct {
	response *http.Response
	err      error
}

func waitForSFTPDownloadResponse(
	ctx context.Context, engine *engineAPI, requestPath string, progress *sftpCLIProgressDisplay,
) (*http.Response, error) {
	if progress == nil {
		return engine.doRaw(ctx, http.MethodGet, requestPath, "", nil)
	}
	result := make(chan sftpRawResponse, 1)
	go func() {
		response, err := engine.doRaw(ctx, http.MethodGet, requestPath, "", nil)
		result <- sftpRawResponse{response: response, err: err}
	}()
	ticker := time.NewTicker(125 * time.Millisecond)
	defer ticker.Stop()
	progress.refresh(ctx)
	for {
		select {
		case answer := <-result:
			progress.refresh(ctx)
			return answer.response, answer.err
		case <-ticker.C:
			progress.refresh(ctx)
		case <-ctx.Done():
			answer := <-result
			if answer.response != nil {
				discardEngineResponse(answer.response)
			}
			return nil, ctx.Err()
		}
	}
}

func newSFTPCLIProgressDisplay(engine *engineAPI, output io.Writer, jsonOutput bool) *sftpCLIProgressDisplay {
	if jsonOutput || engine == nil {
		return nil
	}
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil
	}
	return &sftpCLIProgressDisplay{
		engine: engine, output: output, tracked: make(map[string]string), jobs: make(map[string]sftpCLIProgressJob),
	}
}

func (display *sftpCLIProgressDisplay) track(id, name string) {
	display.mu.Lock()
	display.tracked[id] = name
	display.mu.Unlock()
}

func (display *sftpCLIProgressDisplay) refresh(ctx context.Context) {
	if display == nil {
		return
	}
	display.refreshMu.Lock()
	defer display.refreshMu.Unlock()
	requestContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var queue sftpCLITransferQueue
	if err := display.engine.sendJSON(requestContext, http.MethodGet, "/api/v1/sftp/transfers", nil, &queue); err != nil {
		return
	}
	display.mu.Lock()
	for _, encoded := range queue.Jobs {
		var job sftpCLIProgressJob
		if json.Unmarshal(encoded, &job) != nil || job.ID == "" || !validSFTPDownloadParts(job.DownloadParts) {
			continue
		}
		if _, tracked := display.tracked[job.ID]; tracked {
			display.jobs[job.ID] = job
		}
	}
	lines := sftpProgressLines(display.tracked, display.jobs)
	display.renderLocked(lines)
	display.mu.Unlock()
}

func (display *sftpCLIProgressDisplay) renderLocked(lines []string) {
	if len(lines) == 0 {
		return
	}
	if display.lines > 0 {
		fmt.Fprintf(display.output, "\x1b[%dA", display.lines)
	}
	for _, line := range lines {
		fmt.Fprintf(display.output, "\r\x1b[2K%s\n", line)
	}
	display.lines = len(lines)
}

func sftpProgressLines(tracked map[string]string, jobs map[string]sftpCLIProgressJob) []string {
	ids := make([]string, 0, len(tracked))
	for id := range tracked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0)
	for _, id := range ids {
		job, found := jobs[id]
		if !found || len(job.DownloadParts) == 0 {
			lines = append(lines, fmt.Sprintf("preparing  %s", tracked[id]))
			continue
		}
		parts := append([]sftpCLIDownloadPart(nil), job.DownloadParts...)
		sort.Slice(parts, func(i, j int) bool { return parts[i].Index < parts[j].Index })
		for _, part := range parts {
			lines = append(lines, formatSFTPProgressLine(part, tracked[id]))
		}
	}
	return lines
}

func formatSFTPProgressLine(part sftpCLIDownloadPart, name string) string {
	const width = 24
	percent := 0
	if part.TotalBytes > 0 {
		percent = int(min(int64(100), part.TransferredBytes*100/part.TotalBytes))
	}
	filled := percent * width / 100
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	return fmt.Sprintf("connection %d [%s] %3d%%  %s / %s  %s",
		part.Index+1, bar, percent, sftpProgressBytes(part.TransferredBytes), sftpProgressBytes(part.TotalBytes), compactSFTPProgressName(name))
}

func validSFTPDownloadParts(parts []sftpCLIDownloadPart) bool {
	if len(parts) > sftpcore.MaxLargeFileParallelism {
		return false
	}
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		if part.Index < 0 || part.Index >= sftpcore.MaxLargeFileParallelism || part.TransferredBytes < 0 || part.TotalBytes < 0 || part.TransferredBytes > part.TotalBytes {
			return false
		}
		if _, duplicate := seen[part.Index]; duplicate {
			return false
		}
		seen[part.Index] = struct{}{}
	}
	return true
}

func compactSFTPProgressName(name string) string {
	const maximum = 28
	characters := []rune(name)
	if len(characters) <= maximum {
		return name
	}
	return "…" + string(characters[len(characters)-maximum+1:])
}

func sftpProgressBytes(value int64) string {
	const (
		kib = int64(1 << 10)
		mib = int64(1 << 20)
		gib = int64(1 << 30)
	)
	switch {
	case value >= gib:
		return fmt.Sprintf("%.1f GiB", float64(value)/float64(gib))
	case value >= mib:
		return fmt.Sprintf("%.1f MiB", float64(value)/float64(mib))
	case value >= kib:
		return fmt.Sprintf("%.1f KiB", float64(value)/float64(kib))
	default:
		return fmt.Sprintf("%d B", value)
	}
}

func sftpUploadFile(ctx context.Context, engine *engineAPI, alias, batchID string, file sftpCLIFile, overwrite bool, splitSizeMiB, splitJobs, chunkSizeMiB int) (returnErr error) {
	input, err := os.Open(file.Source)
	if err != nil {
		return err
	}
	defer input.Close()
	fingerprint, err := sftpFingerprint(ctx, input, file.Size)
	if err != nil {
		return err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	jobID, err := sftpIdentifier("put")
	if err != nil {
		return err
	}
	if err := sftpCreateUploadJob(ctx, engine, jobID, batchID, alias, file, overwrite, splitSizeMiB, splitJobs, chunkSizeMiB); err != nil {
		return err
	}
	defer func() {
		if returnErr != nil {
			cancelPath := "/api/v1/sftp/" + url.PathEscape(alias) + "/uploads/" + url.PathEscape(jobID) + "?" + url.Values{"path": {file.Destination}}.Encode()
			cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			response, cancelErr := engine.doRaw(cancelCtx, http.MethodDelete, cancelPath, "", nil)
			if cancelErr == nil {
				discardEngineResponse(response)
			} else {
				_ = sftpJobAction(cancelCtx, engine, jobID, "cancel")
			}
		}
	}()
	if err := sftpStartJob(ctx, engine, jobID); err != nil {
		return err
	}
	basePath := "/api/v1/sftp/" + url.PathEscape(alias) + "/uploads/" + url.PathEscape(jobID)
	var upload sftpCLIUpload
	if err := engine.sendJSON(ctx, http.MethodPost, basePath, map[string]any{
		"path": file.Destination, "size": file.Size, "sourceFingerprint": fingerprint,
	}, &upload); err != nil {
		return err
	}
	if upload.Offset < 0 || upload.Offset > file.Size || upload.ExpectedRevision == "" {
		return errEngineInvalidResponse
	}
	if upload.Parallelism > 1 {
		if !validSFTPUploadRanges(upload.CompletedRanges, upload.Offset, upload.Size, upload.ChunkBytes) {
			return errEngineInvalidResponse
		}
		if err := sftpUploadFileRanges(ctx, engine, input, basePath, file, upload); err != nil {
			return err
		}
		var completed sftpCLITransfer
		return engine.sendJSON(ctx, http.MethodPost, basePath+"/complete", map[string]any{
			"path": file.Destination, "size": file.Size, "expectedRevision": upload.ExpectedRevision,
			"sourceFingerprint": fingerprint,
		}, &completed)
	}
	if _, err := input.Seek(upload.Offset, io.SeekStart); err != nil {
		return err
	}
	buffer := make([]byte, sftpCLIChunkBytes)
	offset := upload.Offset
	for offset < file.Size {
		if err := ctx.Err(); err != nil {
			return err
		}
		limit := file.Size - offset
		if limit > int64(len(buffer)) {
			limit = int64(len(buffer))
		}
		read, err := io.ReadFull(input, buffer[:limit])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return err
		}
		if read == 0 {
			return io.ErrUnexpectedEOF
		}
		appendPath := basePath + "?" + url.Values{
			"path": {file.Destination}, "offset": {fmt.Sprint(offset)}, "total": {fmt.Sprint(file.Size)},
		}.Encode()
		response, err := engine.doRaw(ctx, http.MethodPatch, appendPath, "application/octet-stream", bytes.NewReader(buffer[:read]))
		if err != nil {
			return err
		}
		var appended sftpCLIUpload
		if err := decodeEngineJSONResponse(response, &appended); err != nil {
			return err
		}
		if appended.Offset != offset+int64(read) {
			return errEngineInvalidResponse
		}
		offset = appended.Offset
	}
	var completed sftpCLITransfer
	return engine.sendJSON(ctx, http.MethodPost, basePath+"/complete", map[string]any{
		"path": file.Destination, "size": file.Size, "expectedRevision": upload.ExpectedRevision,
		"sourceFingerprint": fingerprint,
	}, &completed)
}

func validSFTPUploadRanges(ranges []sftpCLIUploadRange, transferred, total, chunkBytes int64) bool {
	if chunkBytes <= 0 || len(ranges) > 65536 || transferred < 0 || transferred > total {
		return false
	}
	var previousEnd, completed int64
	for index, portion := range ranges {
		if portion.Offset < 0 || portion.Size <= 0 || portion.Offset%chunkBytes != 0 || portion.Offset > total ||
			portion.Size > total-portion.Offset || (index > 0 && portion.Offset <= previousEnd) {
			return false
		}
		end := portion.Offset + portion.Size
		if end != total && end%chunkBytes != 0 {
			return false
		}
		previousEnd = end
		completed += portion.Size
	}
	return completed == transferred
}

func sftpUploadFileRanges(ctx context.Context, engine *engineAPI, input *os.File, basePath string, file sftpCLIFile, upload sftpCLIUpload) error {
	ranges := make([]sftpCLIUploadRange, 0)
	for offset := int64(0); offset < file.Size; offset += upload.ChunkBytes {
		size := min(upload.ChunkBytes, file.Size-offset)
		covered := false
		for _, done := range upload.CompletedRanges {
			if offset >= done.Offset && offset+size <= done.Offset+done.Size {
				covered = true
				break
			}
		}
		if !covered {
			ranges = append(ranges, sftpCLIUploadRange{Offset: offset, Size: size})
		}
	}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan sftpCLIUploadRange, len(ranges))
	for _, portion := range ranges {
		work <- portion
	}
	close(work)
	errCh := make(chan error, min(upload.Parallelism, len(ranges)))
	var workers sync.WaitGroup
	for range min(upload.Parallelism, len(ranges)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for portion := range work {
				query := url.Values{"path": {file.Destination}, "offset": {fmt.Sprint(portion.Offset)}, "total": {fmt.Sprint(file.Size)},
					"range": {"true"}, "length": {fmt.Sprint(portion.Size)}}
				response, err := engine.doRaw(workerContext, http.MethodPatch, basePath+"?"+query.Encode(), "application/octet-stream", io.NewSectionReader(input, portion.Offset, portion.Size))
				if err == nil {
					var result sftpCLIUpload
					err = decodeEngineJSONResponse(response, &result)
				}
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
	workers.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}
	return ctx.Err()
}

func sftpCreateDownloadJob(
	ctx context.Context, engine *engineAPI, jobID, batchID, alias string, file sftpCLIFile, splitSizeMiB, splitJobs, chunkSizeMiB int,
) error {
	request := sftpCreateJobRequest(jobID, batchID, alias, "download", file, false)
	if splitSizeMiB > 0 {
		request["largeFileThresholdBytes"] = int64(splitSizeMiB) << 20
	}
	if splitJobs > 0 {
		request["largeFileParallelism"] = splitJobs
	}
	if chunkSizeMiB > 0 {
		request["largeFileChunkBytes"] = int64(chunkSizeMiB) << 20
	}
	var ignored map[string]any
	return engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/transfers", request, &ignored)
}

func sftpCreateUploadJob(ctx context.Context, engine *engineAPI, jobID, batchID, alias string, file sftpCLIFile, overwrite bool, splitSizeMiB, splitJobs, chunkSizeMiB int) error {
	request := sftpCreateJobRequest(jobID, batchID, alias, "upload", file, overwrite)
	if splitSizeMiB > 0 {
		request["largeFileThresholdBytes"] = int64(splitSizeMiB) << 20
	}
	if splitJobs > 0 {
		request["largeFileParallelism"] = splitJobs
	}
	if chunkSizeMiB > 0 {
		request["largeFileChunkBytes"] = int64(chunkSizeMiB) << 20
	}
	var ignored map[string]any
	return engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/transfers", request, &ignored)
}

func sftpCreateJobRequest(jobID, batchID, alias, direction string, file sftpCLIFile, overwrite bool) map[string]any {
	remotePath := file.Destination
	if direction == "download" {
		remotePath = file.Source
	}
	return map[string]any{
		"id": jobID, "batchId": batchID, "batchName": path.Base(remotePath), "batchKind": "file",
		"alias": alias, "sourceAlias": "", "sourcePath": "", "operation": "", "overwrite": overwrite,
		"direction": direction, "kind": "file", "name": path.Base(remotePath),
		"remotePath": remotePath, "totalBytes": file.Size, "lastModified": file.ModifiedUnix,
	}
}

func sftpJobAction(ctx context.Context, engine *engineAPI, jobID, action string) error {
	var ignored map[string]any
	return engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/transfers/"+url.PathEscape(jobID)+"/actions", map[string]string{"action": action}, &ignored)
}

func sftpStartJob(ctx context.Context, engine *engineAPI, jobID string) error {
	for {
		err := sftpJobAction(ctx, engine, jobID, "start")
		if !sftpIsTransferLimit(err) {
			return err
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
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

func sftpIsNotFound(err error) bool {
	var problem engineProblem
	return errors.As(err, &problem) && problem.Code == "sftp_not_found"
}

func sftpIsTransferLimit(err error) bool {
	var problem engineProblem
	return errors.As(err, &problem) && problem.Code == "sftp_transfer_limit"
}

func sftpIdentifier(prefix string) (string, error) {
	contents := make([]byte, 16)
	if _, err := rand.Read(contents); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(contents), nil
}

func sftpFingerprint(ctx context.Context, file *os.File, size int64) (string, error) {
	chunkHashes := make([]byte, 0, int((size+sftpCLIChunkBytes-1)/sftpCLIChunkBytes)*sha256.Size)
	buffer := make([]byte, sftpCLIChunkBytes)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		read, err := file.Read(buffer)
		if read > 0 {
			total += int64(read)
			digest := sha256.Sum256(buffer[:read])
			chunkHashes = append(chunkHashes, digest[:]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if total != size {
		return "", errors.New("local source changed while it was being read")
	}
	summary := make([]byte, 8+len(chunkHashes))
	binary.BigEndian.PutUint64(summary[:8], uint64(size))
	copy(summary[8:], chunkHashes)
	digest := sha256.Sum256(summary)
	return "tree-sha256:" + hex.EncodeToString(digest[:]), nil
}

func finishSFTPSuccess(asJSON bool, result sftpCLIResult, stdout io.Writer) int {
	if asJSON {
		if err := writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: true, Result: result}); err != nil {
			return 1
		}
		return 0
	}
	verb := "transferred"
	if result.DryRun {
		verb = "would transfer"
	}
	fmt.Fprintf(stdout, "%s %d file(s), %d directories, %d bytes", verb, result.Files, result.Directories, result.Bytes)
	if result.Skipped > 0 {
		fmt.Fprintf(stdout, "; skipped %d", result.Skipped)
	}
	if result.Overwritten > 0 {
		fmt.Fprintf(stdout, "; overwritten %d", result.Overwritten)
	}
	fmt.Fprintln(stdout)
	return 0
}

func finishSFTPFailure(asJSON bool, err error, stdout, stderr io.Writer) int {
	failure := classifyCommandFailure(err)
	switch {
	case errors.Is(err, errSFTPRecursiveRequired):
		failure = commandFailure{Kind: "recursive_required", Retryable: false}
	case errors.Is(err, errSFTPExisting):
		failure = commandFailure{Kind: "destination_exists", Retryable: false}
	case errors.Is(err, errSFTPTypeMismatch):
		failure = commandFailure{Kind: "type_mismatch", Retryable: false}
	case errors.Is(err, errSFTPUnsupportedLocal):
		failure = commandFailure{Kind: "unsupported_file_type", Retryable: false}
	case errors.Is(err, errSFTPRemotePath):
		failure = commandFailure{Kind: "invalid_remote_path", Retryable: false}
	case errors.Is(err, errSFTPRecursiveLimit):
		failure = commandFailure{Kind: "recursive_limit", Retryable: false}
	case errors.Is(err, fs.ErrNotExist):
		failure = commandFailure{Kind: "local_not_found", Retryable: false}
	}
	exit := 1
	if errors.Is(err, context.Canceled) {
		exit = 130
	}
	if asJSON {
		_ = writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: false, Failure: &failure})
		return exit
	}
	switch failure.Kind {
	case "engine_not_running":
		fmt.Fprintln(stderr, "sshc: no engine is running; start the desktop app or run sshc engine in another terminal")
	case "vault_missing":
		fmt.Fprintln(stderr, "sshc: no vault exists; run sshc vault create")
	case "vault_locked":
		fmt.Fprintln(stderr, "sshc: the vault is locked; run sshc vault unlock")
	case "recursive_required":
		fmt.Fprintln(stderr, "sshc: the source is a directory; rerun with --recursive")
	case "destination_exists":
		fmt.Fprintln(stderr, "sshc: a destination file exists; rerun with --overwrite or --skip-existing")
	case "type_mismatch":
		fmt.Fprintln(stderr, "sshc: a file and directory occupy the same destination path")
	case "unsupported_file_type":
		fmt.Fprintln(stderr, "sshc: symlinks and special files are not transferred")
	case "invalid_remote_path":
		fmt.Fprintln(stderr, "sshc: remote paths must be absolute POSIX paths")
	case "recursive_limit":
		fmt.Fprintf(stderr, "sshc: %v; choose a narrower source or raise the recursive limit explicitly\n", err)
	case "local_not_found":
		fmt.Fprintln(stderr, "sshc: the local source does not exist")
	case "canceled":
		fmt.Fprintln(stderr, "sshc: sftp transfer was canceled")
	default:
		fmt.Fprintf(stderr, "sshc: sftp failed (%s)\n", failure.Kind)
	}
	return exit
}

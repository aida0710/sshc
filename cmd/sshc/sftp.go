package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"

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

type sftpCLISettingsResult struct {
	SplitSizeMiB int64 `json:"splitSizeMiB"`
	SplitJobs    int   `json:"splitJobs"`
	ChunkSizeMiB int64 `json:"chunkSizeMiB"`
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

func sftpIsNotFound(err error) bool {
	var problem engineProblem
	return errors.As(err, &problem) && problem.Code == "sftp_not_found"
}

func sftpIsTransferLimit(err error) bool {
	var problem engineProblem
	return errors.As(err, &problem) && problem.Code == "sftp_transfer_limit"
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

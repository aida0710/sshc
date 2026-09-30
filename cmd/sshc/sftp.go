package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"time"

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

	// CLI は分割転送の大きさを MiB で受け取る。範囲の正本は engine の sftpcore の定数で、
	// ここでは単位を MiB に直すだけにする。engine の範囲を変えたときに、CLI が正当な
	// 値や engine の応答を断らないようにするためである。
	sftpCLIMinSplitSizeMiB = int(sftpcore.MinLargeFileThreshold >> 20)
	sftpCLIMaxSplitSizeMiB = int(sftpcore.MaxLargeFileThreshold >> 20)
	sftpCLIMinChunkSizeMiB = int(sftpcore.MinLargeFileChunkBytes >> 20)
	sftpCLIMaxChunkSizeMiB = int(sftpcore.MaxLargeFileChunkBytes >> 20)
	// sftpCLIMaxClearCompletedAfterSeconds は、engine が返す完了項目の自動消去時間の上限を秒で表す。
	sftpCLIMaxClearCompletedAfterSeconds = int(sftpcore.MaxClearCompletedAfter / time.Second)
)

var (
	errSFTPRecursiveRequired = errors.New("recursive transfer requires --recursive")
	errSFTPExisting          = errors.New("destination already exists")
	errSFTPTypeMismatch      = errors.New("source and destination types do not match")
	errSFTPUnsupportedLocal  = errors.New("the source is neither a file nor a directory")
	// The remote directory a single file was to be put into does not exist. A
	// recursive put creates the directories it needs; a single file does not
	// guess at a directory the person may have mistyped.
	errSFTPRemoteDirectoryMissing = errors.New("the remote directory does not exist")
	errSFTPMissingRevision        = errors.New("download response has no revision")
	errSFTPRemotePath             = errors.New("remote path must be absolute")
	errSFTPRecursiveLimit         = errors.New("recursive download safety limit exceeded")
)

type sftpCLIEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModifiedAt string `json:"modifiedAt"`
	Revision   string `json:"revision"`
	// Symlinks only: where the link points and what kind of entry that is.
	// The engine follows the link for reads, so a link to a file transfers
	// as that file and a link to a directory lists as that directory.
	LinkTarget string `json:"linkTarget,omitempty"`
	TargetType string `json:"targetType,omitempty"`
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

func runSFTP(ctx context.Context, called sftpInvocation, environment commandEnvironment) int {
	stdout, stderr := environment.stdout, environment.stderr
	engine, err := openEngineAPI(ctx, environment.stateDir, environment.client)
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
		confirmed, code := confirmAction(ctx, called.Yes, "Continue? [y/N] ", environment.confirm, stderr)
		if code != 0 {
			if called.JSON {
				failure := commandFailure{Kind: "confirmation_required", Retryable: false}
				if errors.Is(ctx.Err(), context.Canceled) {
					failure = commandFailure{Kind: "canceled", Retryable: true}
				}
				_ = writeCommandFailure(stdout, failure)
			}
			return code
		}
		if !confirmed {
			fmt.Fprintln(stderr, "sshc: canceled")
			if called.JSON {
				failure := commandFailure{Kind: "confirmation_declined", Retryable: false}
				_ = writeCommandFailure(stdout, failure)
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
		if err := writeCommandSuccess(stdout, result); err != nil {
			return exitFailure
		}
		return 0
	}
	fmt.Fprintf(stdout, "split-size  %d MiB\nsplit-jobs  %d\nchunk-size  %d MiB\n", result.SplitSizeMiB, result.SplitJobs, result.ChunkSizeMiB)
	return 0
}

func validSFTPCLITransferSettings(settings sftpCLITransferQueue) bool {
	return settings.MaxConcurrent >= 1 && settings.MaxConcurrent <= sftpcore.MaxTransferConcurrency &&
		settings.ClearCompletedAfterSeconds >= 0 && settings.ClearCompletedAfterSeconds <= sftpCLIMaxClearCompletedAfterSeconds &&
		settings.LargeFileThresholdBytes >= sftpcore.MinLargeFileThreshold && settings.LargeFileThresholdBytes <= sftpcore.MaxLargeFileThreshold &&
		settings.LargeFileParallelism >= 1 && settings.LargeFileParallelism <= sftpcore.MaxLargeFileParallelism &&
		settings.LargeFileChunkBytes >= sftpcore.MinLargeFileChunkBytes && settings.LargeFileChunkBytes <= sftpcore.MaxLargeFileChunkBytes
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
		if err := writeCommandSuccess(stdout, result); err != nil {
			return exitFailure
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
	return finishCommandFailure(commandFailureReport{
		cause: err, failure: classifySFTPFailure(err), asJSON: asJSON, stdout: stdout, stderr: stderr,
		writeHuman: func(stderr io.Writer, failure commandFailure) { writeHumanSFTPFailure(stderr, failure, err) },
	})
}

// classifySFTPFailure は、SFTP の転送に固有の失敗を種別に分け、ほかは共通の分け方に任せる。
func classifySFTPFailure(err error) commandFailure {
	switch {
	case errors.Is(err, errSFTPRecursiveRequired):
		return commandFailure{Kind: "recursive_required", Retryable: false}
	case errors.Is(err, errSFTPExisting):
		return commandFailure{Kind: "destination_exists", Retryable: false}
	case errors.Is(err, errSFTPTypeMismatch):
		return commandFailure{Kind: "type_mismatch", Retryable: false}
	case errors.Is(err, errSFTPUnsupportedLocal):
		return commandFailure{Kind: "unsupported_file_type", Retryable: false}
	case errors.Is(err, errSFTPRemoteDirectoryMissing):
		return commandFailure{Kind: "remote_directory_missing", Retryable: false}
	case errors.Is(err, errSFTPRemotePath):
		return commandFailure{Kind: "invalid_remote_path", Retryable: false}
	case errors.Is(err, errSFTPRecursiveLimit):
		return commandFailure{Kind: "recursive_limit", Retryable: false}
	// handoff が無い（engine が動いていない）ことも fs.ErrNotExist なので、手元の
	// 転送元が無いことと取り違えない。
	case errors.Is(err, fs.ErrNotExist) && !isEngineNotRunning(err):
		return commandFailure{Kind: "local_not_found", Retryable: false}
	default:
		return classifyCommandFailure(err)
	}
}

// writeHumanSFTPFailure は、SFTP の転送に固有の失敗の種別を人向けの文で書く。一部の文は、
// どのファイルかを示すために元の失敗 cause をそのまま含める。
func writeHumanSFTPFailure(stderr io.Writer, failure commandFailure, cause error) {
	switch failure.Kind {
	case "recursive_required":
		fmt.Fprintln(stderr, "sshc: the source is a directory; rerun with --recursive")
	case "destination_exists":
		fmt.Fprintln(stderr, "sshc: a destination file exists; rerun with --overwrite or --skip-existing")
	case "type_mismatch":
		fmt.Fprintln(stderr, "sshc: a file and directory occupy the same destination path")
	case "unsupported_file_type":
		fmt.Fprintf(stderr, "sshc: %v; only files and directories (and links to them) are transferred\n", cause)
	case "remote_directory_missing":
		fmt.Fprintf(stderr, "sshc: %v; create it first or put a directory with --recursive\n", cause)
	case "invalid_remote_path":
		fmt.Fprintln(stderr, "sshc: remote paths must be absolute POSIX paths")
	case "recursive_limit":
		fmt.Fprintf(stderr, "sshc: %v; choose a narrower source or raise the recursive limit explicitly\n", cause)
	case "local_not_found":
		fmt.Fprintln(stderr, "sshc: the local source does not exist")
	case "canceled":
		fmt.Fprintln(stderr, "sshc: sftp transfer was canceled")
	default:
		fmt.Fprintf(stderr, "sshc: sftp failed (%s)\n", failure.Kind)
	}
}

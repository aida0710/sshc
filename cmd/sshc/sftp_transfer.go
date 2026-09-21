package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	sftpcore "sshc/internal/sftp"
)

func reportSkippedEntries(plan sftpCLIPlan, stderr io.Writer) {
	for _, skipped := range plan.SkippedPaths {
		fmt.Fprintf(stderr, "skip %s: %s\n", skipped.Path, skipped.Reason)
	}
}

// sftpTransferBatch は、1 回の get／put で全ファイルに共通する条件。ファイルごとの
// worker は、これと自分のファイルだけを受け取る。
type sftpTransferBatch struct {
	engine  *engineAPI
	alias   string
	batchID string
	// overwrite は put だけが使う。get は engine 側の既定（上書きしない）のまま。
	overwrite bool
	// 大きなファイルの分割は 0 なら engine の設定に任せる。
	splitSizeMiB int
	splitJobs    int
	chunkSizeMiB int
	// progress は get だけが持つ。nil なら表示しない。
	progress *sftpCLIProgressDisplay
}

func newSFTPTransferBatch(engine *engineAPI, plan sftpCLIPlan, called sftpInvocation) (sftpTransferBatch, error) {
	batchID, err := sftpIdentifier("batch")
	if err != nil {
		return sftpTransferBatch{}, err
	}
	return sftpTransferBatch{
		engine: engine, alias: plan.Alias, batchID: batchID,
		splitSizeMiB: called.SplitSizeMiB, splitJobs: called.SplitJobs, chunkSizeMiB: called.ChunkSizeMiB,
	}, nil
}

func executeSFTPGet(ctx context.Context, engine *engineAPI, plan sftpCLIPlan, called sftpInvocation, stderr io.Writer) error {
	reportSkippedEntries(plan, stderr)
	for _, directory := range plan.Directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	batch, err := newSFTPTransferBatch(engine, plan, called)
	if err != nil {
		return err
	}
	files := transferableSFTPFiles(plan.Files, called.SkipExisting)
	for _, file := range files {
		fmt.Fprintf(stderr, "get  %s:%s -> %s\n", plan.Alias, file.Source, file.Destination)
	}
	batch.progress = newSFTPCLIProgressDisplay(engine, stderr, called.JSON)
	return runSFTPFileWorkers(ctx, files, called.Jobs, func(workerContext context.Context, file sftpCLIFile) error {
		return sftpDownloadFile(workerContext, batch, file)
	})
}

func executeSFTPPut(ctx context.Context, engine *engineAPI, plan sftpCLIPlan, called sftpInvocation, stderr io.Writer) error {
	reportSkippedEntries(plan, stderr)
	for _, directory := range plan.Directories {
		if err := sftpEnsureRemoteDirectory(ctx, engine, plan.Alias, directory); err != nil {
			return err
		}
	}
	batch, err := newSFTPTransferBatch(engine, plan, called)
	if err != nil {
		return err
	}
	files := transferableSFTPFiles(plan.Files, called.SkipExisting)
	for _, file := range files {
		fmt.Fprintf(stderr, "put  %s -> %s:%s\n", file.Source, plan.Alias, file.Destination)
	}
	batch.overwrite = called.Overwrite
	return runSFTPFileWorkers(ctx, files, called.Jobs, func(workerContext context.Context, file sftpCLIFile) error {
		return sftpUploadFile(workerContext, batch, file)
	})
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

func sftpDownloadFile(ctx context.Context, batch sftpTransferBatch, file sftpCLIFile) (returnErr error) {
	engine, alias, progress := batch.engine, batch.alias, batch.progress
	jobID, err := sftpIdentifier("get")
	if err != nil {
		return err
	}
	if err := batch.createJob(ctx, jobID, "download", file); err != nil {
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
	// The copy keeps the time the remote file had, as WinSCP does by default.
	if file.ModifiedUnix > 0 {
		modified := time.UnixMilli(file.ModifiedUnix)
		if err := os.Chtimes(temporaryName, modified, modified); err != nil {
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

func sftpUploadFile(ctx context.Context, batch sftpTransferBatch, file sftpCLIFile) (returnErr error) {
	engine, alias := batch.engine, batch.alias
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
	if err := batch.createJob(ctx, jobID, "upload", file); err != nil {
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

// createJob は engine の転送キューに 1 ファイルのジョブを登録する。direction は
// "download" か "upload" で、リモート側のパスはそれに応じて file の Source か
// Destination になる。
func (b sftpTransferBatch) createJob(ctx context.Context, jobID, direction string, file sftpCLIFile) error {
	remotePath := file.Destination
	if direction == "download" {
		remotePath = file.Source
	}
	request := map[string]any{
		"id": jobID, "batchId": b.batchID, "batchName": path.Base(remotePath), "batchKind": "file",
		"alias": b.alias, "sourceAlias": "", "sourcePath": "", "operation": "", "overwrite": b.overwrite,
		"direction": direction, "kind": "file", "name": path.Base(remotePath),
		"remotePath": remotePath, "totalBytes": file.Size, "lastModified": file.ModifiedUnix,
	}
	if b.splitSizeMiB > 0 {
		request["largeFileThresholdBytes"] = int64(b.splitSizeMiB) << 20
	}
	if b.splitJobs > 0 {
		request["largeFileParallelism"] = b.splitJobs
	}
	if b.chunkSizeMiB > 0 {
		request["largeFileChunkBytes"] = int64(b.chunkSizeMiB) << 20
	}
	var ignored map[string]any
	return b.engine.sendJSON(ctx, http.MethodPost, "/api/v1/sftp/transfers", request, &ignored)
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

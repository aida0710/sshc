package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/validate"
)

type SFTPHandlers struct {
	Service   *sshcSFTP.Service
	Transfers *sshcSFTP.TransferManager
	// Config は転送キューの設定を metadata.json へ残す。nil なら engine の
	// process が生きているあいだだけ効く。
	Config  *application.Service
	Actions ActionHandlers
}

type sftpEntry struct {
	Name       string             `json:"name"`
	Path       string             `json:"path"`
	Type       sshcSFTP.EntryType `json:"type"`
	Size       int64              `json:"size"`
	Mode       string             `json:"mode"`
	ModifiedAt string             `json:"modifiedAt"`
	Revision   string             `json:"revision"`
}

type sftpListingResponse struct {
	Path    string      `json:"path"`
	Entries []sftpEntry `json:"entries"`
}

type sftpSearchResponse struct {
	Path      string      `json:"path"`
	Query     string      `json:"query"`
	Truncated bool        `json:"truncated"`
	Entries   []sftpEntry `json:"entries"`
}

type sftpTextFileResponse struct {
	Entry    sftpEntry `json:"entry"`
	Contents string    `json:"contents"`
	Revision string    `json:"revision"`
}

type sftpSaveTextRequest struct {
	Contents         string `json:"contents"`
	ExpectedRevision string `json:"expectedRevision"`
}

type sftpMkdirType string

const sftpMkdirDirectory sftpMkdirType = "directory"

type sftpMkdirRequest struct {
	Path string        `json:"path"`
	Type sftpMkdirType `json:"type"`
}

type sftpRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type sftpChmodRequest struct {
	Path             string `json:"path"`
	Mode             string `json:"mode"`
	ExpectedRevision string `json:"expectedRevision"`
}

type sftpTransferResponse struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Revision string `json:"revision"`
}

type changedResponse struct {
	Changed bool `json:"changed"`
}

func registerSFTPRoutes(engine *echo.Echo, handlers SFTPHandlers) {
	engine.GET("/api/v1/sftp/transfers", handlers.ListTransfers)
	engine.POST("/api/v1/sftp/transfers", handlers.CreateTransfer)
	engine.DELETE("/api/v1/sftp/transfers/finished", handlers.ClearFinishedTransfers)
	engine.DELETE("/api/v1/sftp/transfers/:id", handlers.RemoveTransfer)
	engine.PUT("/api/v1/sftp/transfers/settings", handlers.UpdateTransferSettings)
	engine.POST("/api/v1/sftp/transfers/:id/queue-position", handlers.MoveTransfer)
	engine.POST("/api/v1/sftp/transfers/:id/actions", handlers.UpdateTransfer)
	engine.POST("/api/v1/sftp/transfers/:id/download-checkpoint", handlers.CheckpointDownload)
	engine.GET("/api/v1/sftp/compare", handlers.CompareDirectories)
	engine.GET("/api/v1/sftp/:alias/entries", handlers.List)
	engine.POST("/api/v1/sftp/:alias/entries", handlers.Mkdir)
	engine.GET("/api/v1/sftp/:alias/preview", handlers.Preview)
	engine.GET("/api/v1/sftp/:alias/search", handlers.Search)
	engine.GET("/api/v1/sftp/:alias/text", handlers.ReadText)
	engine.PUT("/api/v1/sftp/:alias/text", handlers.SaveText)
	engine.GET("/api/v1/sftp/:alias/download", handlers.Download)
	engine.GET("/api/v1/sftp/:alias/archive", handlers.DownloadArchive)
	engine.POST("/api/v1/sftp/:alias/uploads/:id", handlers.StartUpload)
	engine.PATCH("/api/v1/sftp/:alias/uploads/:id", handlers.AppendUpload)
	engine.POST("/api/v1/sftp/:alias/uploads/:id/complete", handlers.CompleteUpload)
	engine.DELETE("/api/v1/sftp/:alias/uploads/:id", handlers.CancelUpload)
	engine.PATCH("/api/v1/sftp/:alias/entry", handlers.Rename)
	engine.DELETE("/api/v1/sftp/:alias/entry", handlers.Delete)
	engine.PATCH("/api/v1/sftp/:alias/mode", handlers.Chmod)
}

func describeSFTPEntry(entry sshcSFTP.Entry) sftpEntry {
	return sftpEntry{
		Name: entry.Name, Path: entry.Path, Type: entry.Type, Size: entry.Size,
		Mode: entry.Mode.String(), ModifiedAt: entry.ModifiedAt.UTC().Format(time.RFC3339Nano), Revision: entry.Revision,
	}
}

func describeSFTPAPIEntry(entry sshcSFTP.Entry) api.SFTPEntry {
	return api.SFTPEntry{
		Name: entry.Name, Path: entry.Path, Type: api.SFTPEntryType(entry.Type), Size: entry.Size,
		Mode: entry.Mode.String(), ModifiedAt: entry.ModifiedAt.UTC(), Revision: entry.Revision,
	}
}

func sftpProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, validate.ErrUnsafeAlias):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	case errors.Is(err, fs.ErrNotExist):
		return problem(c, http.StatusNotFound, "sftp_not_found")
	case errors.Is(err, sshcSFTP.ErrTransferNotFound):
		return problem(c, http.StatusNotFound, "sftp_transfer_not_found")
	case errors.Is(err, sshcSFTP.ErrInvalidAlias), errors.Is(err, sshcSFTP.ErrInvalidPath), errors.Is(err, sshcSFTP.ErrRootOperation), errors.Is(err, sshcSFTP.ErrRevisionRequired), errors.Is(err, sshcSFTP.ErrInvalidTransfer), errors.Is(err, sshcSFTP.ErrInvalidQuery):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, sshcSFTP.ErrConflict), errors.Is(err, sshcSFTP.ErrOffsetMismatch), errors.Is(err, sshcSFTP.ErrUploadIncomplete):
		return problem(c, http.StatusConflict, "sftp_conflict")
	case errors.Is(err, sshcSFTP.ErrTransferState):
		return problem(c, http.StatusConflict, "sftp_transfer_state")
	case errors.Is(err, sshcSFTP.ErrTransferLimit):
		return problem(c, http.StatusConflict, "sftp_transfer_limit")
	case errors.Is(err, sshcSFTP.ErrAlreadyExists):
		return problem(c, http.StatusConflict, "sftp_exists")
	case errors.Is(err, sshcSFTP.ErrTextTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_text_too_large")
	case errors.Is(err, sshcSFTP.ErrTransferTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_transfer_too_large")
	case errors.Is(err, sshcSFTP.ErrPreviewTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_preview_too_large")
	case errors.Is(err, sshcSFTP.ErrPreviewType):
		return problem(c, http.StatusUnsupportedMediaType, "sftp_preview_type")
	case errors.Is(err, sshcSFTP.ErrNotUTF8):
		return problem(c, http.StatusUnprocessableEntity, "sftp_not_utf8")
	case errors.Is(err, sshcSFTP.ErrNotRegularFile), errors.Is(err, sshcSFTP.ErrNotDirectory):
		return problem(c, http.StatusUnprocessableEntity, "sftp_wrong_type")
	case errors.Is(err, sshcSFTP.ErrUnsupportedEntry):
		return problem(c, http.StatusUnprocessableEntity, "sftp_unsupported_entry")
	case errors.Is(err, sshcSFTP.ErrCompareLimit):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_compare_limit")
	default:
		return problem(c, http.StatusBadGateway, "sftp_failed")
	}
}

type sftpTransferJobResponse struct {
	ID                string                           `json:"id"`
	BatchID           string                           `json:"batchId"`
	BatchName         string                           `json:"batchName"`
	BatchKind         sshcSFTP.TransferKind            `json:"batchKind"`
	Alias             string                           `json:"alias"`
	SourceAlias       string                           `json:"sourceAlias"`
	SourcePath        string                           `json:"sourcePath"`
	Operation         sshcSFTP.RemoteTransferOperation `json:"operation"`
	Direction         sshcSFTP.TransferDirection       `json:"direction"`
	Kind              sshcSFTP.TransferKind            `json:"kind"`
	Name              string                           `json:"name"`
	RemotePath        string                           `json:"remotePath"`
	TotalBytes        int64                            `json:"totalBytes"`
	TransferredBytes  int64                            `json:"transferredBytes"`
	BytesPerSecond    float64                          `json:"bytesPerSecond"`
	RemainingSeconds  int64                            `json:"remainingSeconds"`
	Status            sshcSFTP.TransferJobStatus       `json:"status"`
	AllowedActions    []sshcSFTP.TransferControlAction `json:"allowedActions"`
	Attempt           int                              `json:"attempt"`
	Problem           string                           `json:"problem"`
	LastModified      int64                            `json:"lastModified"`
	ExpectedRevision  string                           `json:"expectedRevision"`
	SourceFingerprint string                           `json:"sourceFingerprint"`
	Overwrite         bool                             `json:"overwrite"`
	DownloadRevision  string                           `json:"downloadRevision"`
	CreatedAt         string                           `json:"createdAt"`
	UpdatedAt         string                           `json:"updatedAt"`
}

func describeTransferJob(job sshcSFTP.TransferJob) sftpTransferJobResponse {
	return sftpTransferJobResponse{
		ID: job.ID, BatchID: job.BatchID, BatchName: job.BatchName, BatchKind: job.BatchKind,
		Alias: job.Alias, SourceAlias: job.SourceAlias, SourcePath: job.SourcePath,
		Operation: job.Operation, Direction: job.Direction,
		Kind: job.Kind, Name: job.Name, RemotePath: job.RemotePath, TotalBytes: job.TotalBytes,
		TransferredBytes: job.TransferredBytes, BytesPerSecond: job.BytesPerSecond,
		RemainingSeconds: job.RemainingSeconds, Status: job.Status,
		AllowedActions: sshcSFTP.AllowedTransferActions(job), Attempt: job.Attempt,
		Problem: job.Problem, LastModified: job.LastModified,
		ExpectedRevision: job.ExpectedRevision, SourceFingerprint: job.SourceFingerprint,
		Overwrite: job.Overwrite, DownloadRevision: job.DownloadRevision,
		CreatedAt: job.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: job.UpdatedAt.Format(time.RFC3339Nano),
	}
}

type sftpTransferJobListResponse struct {
	MaxConcurrent           int   `json:"maxConcurrent"`
	LargeFileThresholdBytes int64 `json:"largeFileThresholdBytes"`
	LargeFileParallelism    int   `json:"largeFileParallelism"`
	LargeFileChunkBytes     int64 `json:"largeFileChunkBytes"`
	// 0 は自動消去なしである。
	ClearCompletedAfterSeconds int `json:"clearCompletedAfterSeconds"`
	// 停止中は待機の job を新しく開始しない。
	ProcessingStopped bool                      `json:"processingStopped"`
	Jobs              []sftpTransferJobResponse `json:"jobs"`
}

type sftpTransferSettingsRequest struct {
	MaxConcurrent              int   `json:"maxConcurrent"`
	ClearCompletedAfterSeconds int   `json:"clearCompletedAfterSeconds"`
	ProcessingStopped          bool  `json:"processingStopped"`
	LargeFileThresholdBytes    int64 `json:"largeFileThresholdBytes"`
	LargeFileParallelism       int   `json:"largeFileParallelism"`
	LargeFileChunkBytes        int64 `json:"largeFileChunkBytes"`
}

type sftpTransferQueueMoveRequest struct {
	Move sshcSFTP.TransferQueueMove `json:"move"`
}

type sftpCreateTransferJobRequest struct {
	ID                      string                           `json:"id"`
	BatchID                 string                           `json:"batchId"`
	BatchName               string                           `json:"batchName"`
	BatchKind               sshcSFTP.TransferKind            `json:"batchKind"`
	Alias                   string                           `json:"alias"`
	SourceAlias             string                           `json:"sourceAlias"`
	SourcePath              string                           `json:"sourcePath"`
	Operation               sshcSFTP.RemoteTransferOperation `json:"operation"`
	Overwrite               bool                             `json:"overwrite"`
	Direction               sshcSFTP.TransferDirection       `json:"direction"`
	Kind                    sshcSFTP.TransferKind            `json:"kind"`
	Name                    string                           `json:"name"`
	RemotePath              string                           `json:"remotePath"`
	TotalBytes              int64                            `json:"totalBytes"`
	LastModified            int64                            `json:"lastModified"`
	LargeFileThresholdBytes int64                            `json:"largeFileThresholdBytes,omitempty"`
	LargeFileParallelism    int                              `json:"largeFileParallelism,omitempty"`
	LargeFileChunkBytes     int64                            `json:"largeFileChunkBytes,omitempty"`
}

type sftpTransferJobActionRequest struct {
	Action           sshcSFTP.TransferJobAction `json:"action"`
	TransferredBytes *int64                     `json:"transferredBytes,omitempty"`
	TotalBytes       *int64                     `json:"totalBytes,omitempty"`
	Problem          string                     `json:"problem,omitempty"`
	ResetProgress    bool                       `json:"resetProgress,omitempty"`
}

type sftpDownloadCheckpointRequest struct {
	Offset   int64  `json:"offset"`
	Revision string `json:"revision"`
}

func (h SFTPHandlers) ListTransfers(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.describeTransferQueue())
}

func (h SFTPHandlers) describeTransferQueue() sftpTransferJobListResponse {
	jobs := h.Transfers.ListJobs()
	described := make([]sftpTransferJobResponse, 0, len(jobs))
	for _, job := range jobs {
		described = append(described, describeTransferJob(job))
	}
	return sftpTransferJobListResponse{
		MaxConcurrent:              h.Transfers.MaxConcurrent(),
		LargeFileThresholdBytes:    h.Transfers.LargeFileThreshold(),
		LargeFileParallelism:       h.Transfers.LargeFileParallelism(),
		LargeFileChunkBytes:        h.Transfers.LargeFileChunkBytes(),
		ClearCompletedAfterSeconds: int(h.Transfers.ClearCompletedAfter() / time.Second),
		ProcessingStopped:          h.Transfers.ProcessingStopped(),
		Jobs:                       described,
	}
}

// UpdateTransferSettings は、engine が持つ転送キューの設定を差し替え、
// metadata.json へ残す。転送は engine の資源なので、設定も browser ごとでは
// なく engine にひとつだけ置く。
func (h SFTPHandlers) UpdateTransferSettings(c *echo.Context) error {
	var body sftpTransferSettingsRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Transfers.SetTransferSettings(
		body.MaxConcurrent, time.Duration(body.ClearCompletedAfterSeconds)*time.Second, body.ProcessingStopped,
		body.LargeFileThresholdBytes, body.LargeFileParallelism, body.LargeFileChunkBytes,
	); err != nil {
		return sftpProblem(c, err)
	}
	if h.Config != nil {
		if _, err := h.Config.SetFileTransferSettings(application.FileTransferSettings{
			MaxConcurrent:              body.MaxConcurrent,
			ClearCompletedAfterSeconds: body.ClearCompletedAfterSeconds,
			ProcessingStopped:          body.ProcessingStopped,
			LargeFileThresholdBytes:    body.LargeFileThresholdBytes,
			LargeFileParallelism:       body.LargeFileParallelism,
			LargeFileChunkBytes:        body.LargeFileChunkBytes,
		}); err != nil {
			return serviceProblem(c, err)
		}
	}
	return c.JSON(http.StatusOK, h.describeTransferQueue())
}

// MoveTransfer は、待機中の job を待機列の中で入れ替える。
func (h SFTPHandlers) MoveTransfer(c *echo.Context) error {
	var body sftpTransferQueueMoveRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Transfers.MoveQueuedJob(c.Param("id"), body.Move); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, h.describeTransferQueue())
}

func (h SFTPHandlers) CreateTransfer(c *echo.Context) error {
	var body sftpCreateTransferJobRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.CreateJob(sshcSFTP.CreateTransferJob{
		ID: body.ID, BatchID: body.BatchID, BatchName: body.BatchName, BatchKind: body.BatchKind, Alias: body.Alias,
		SourceAlias: body.SourceAlias, SourcePath: body.SourcePath, Operation: body.Operation,
		Overwrite: body.Overwrite,
		Direction: body.Direction, Kind: body.Kind,
		Name: body.Name, RemotePath: body.RemotePath, TotalBytes: body.TotalBytes, LastModified: body.LastModified,
		LargeFileThresholdBytes: body.LargeFileThresholdBytes, LargeFileParallelism: body.LargeFileParallelism,
		LargeFileChunkBytes: body.LargeFileChunkBytes,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	if job.Direction == sshcSFTP.TransferRemote {
		h.Transfers.ScheduleRemoteJob(job.ID)
	}
	return c.JSON(http.StatusCreated, describeTransferJob(job))
}

func (h SFTPHandlers) CompareDirectories(c *echo.Context) error {
	comparison, err := h.Service.CompareDirectories(
		c.Request().Context(), c.QueryParam("leftAlias"), c.QueryParam("leftPath"),
		c.QueryParam("rightAlias"), c.QueryParam("rightPath"),
	)
	if err != nil {
		return sftpProblem(c, err)
	}
	entries := make([]api.SFTPDirectoryDifference, 0, len(comparison.Entries))
	for _, difference := range comparison.Entries {
		entry := api.SFTPDirectoryDifference{
			RelativePath: difference.RelativePath,
			Status:       api.SFTPDirectoryDifferenceStatus(difference.Status),
		}
		if difference.Left != nil {
			described := describeSFTPAPIEntry(*difference.Left)
			entry.Left = &described
		}
		if difference.Right != nil {
			described := describeSFTPAPIEntry(*difference.Right)
			entry.Right = &described
		}
		entries = append(entries, entry)
	}
	return c.JSON(http.StatusOK, api.SFTPDirectoryComparison{
		LeftPath: comparison.LeftPath, RightPath: comparison.RightPath, Entries: entries,
	})
}

func (h SFTPHandlers) ClearFinishedTransfers(c *echo.Context) error {
	h.Transfers.ClearFinished()
	return c.NoContent(http.StatusNoContent)
}

func (h SFTPHandlers) RemoveTransfer(c *echo.Context) error {
	if err := h.Transfers.RemoveJob(c.Param("id")); err != nil {
		return sftpProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h SFTPHandlers) UpdateTransfer(c *echo.Context) error {
	var body sftpTransferJobActionRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.UpdateJobFromClient(c.Param("id"), sshcSFTP.UpdateTransferJob{
		Action: body.Action, TransferredBytes: body.TransferredBytes,
		TotalBytes: body.TotalBytes, Problem: body.Problem, ResetProgress: body.ResetProgress,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeTransferJob(job))
}

func (h SFTPHandlers) CheckpointDownload(c *echo.Context) error {
	var body sftpDownloadCheckpointRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.AcknowledgeDownload(c.Param("id"), body.Offset, body.Revision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeTransferJob(job))
}

func (h SFTPHandlers) List(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	listing, err := h.Service.ListDirectory(c.Request().Context(), c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]sftpEntry, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		described = append(described, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, sftpListingResponse{Path: listing.Path, Entries: described})
}

// Search は、あるディレクトリ配下の名前一致を返す。歩き切れなかったときは
// truncated がそう言う。
func (h SFTPHandlers) Search(c *echo.Context) error {
	found, err := h.Service.Search(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), c.QueryParam("query"))
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]sftpEntry, 0, len(found.Entries))
	for _, entry := range found.Entries {
		described = append(described, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, sftpSearchResponse{
		Path: found.Path, Query: found.Query, Truncated: found.Truncated, Entries: described,
	})
}

func (h SFTPHandlers) ReadText(c *echo.Context) error {
	file, err := h.Service.ReadText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{
		Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision,
	})
}

// Preview は、詳細モーダルが描く画像そのものを返す。
//
// 名乗る型は Service が中身から決めたものだけであり、SFTP server の申告でも
// 拡張子でもない。X-Content-Type-Options は Security.Middleware が全応答へ
// 付けているので、ここで名乗った型より先へブラウザが推測することはない。
func (h SFTPHandlers) Preview(c *echo.Context) error {
	preview, err := h.Service.ReadPreview(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("ETag", strconv.Quote(preview.Revision))
	return c.Blob(http.StatusOK, preview.ContentType, preview.Contents)
}

func (h SFTPHandlers) SaveText(c *echo.Context) error {
	var body sftpSaveTextRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	file, err := h.Transfers.SaveText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), body.Contents, body.ExpectedRevision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, sftpTextFileResponse{
		Entry: describeSFTPEntry(file.Entry), Contents: file.Contents, Revision: file.Revision,
	})
}

func (h SFTPHandlers) Mkdir(c *echo.Context) error {
	var body sftpMkdirRequest
	if err := decodeJSON(c, &body); err != nil || body.Type != sftpMkdirDirectory {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	entry, err := h.Service.Mkdir(c.Request().Context(), alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Rename(c *echo.Context) error {
	var body sftpRenameRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.From, body.To)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	entry, err := h.Service.Rename(c.Request().Context(), alias, body.From, body.To)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Download(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	job, done, err := h.Transfers.StartDownloadDataPlane(c.QueryParam("jobId"), c.Param("alias"), remotePath, sshcSFTP.TransferFile)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer done()
	name := path.Base(remotePath)
	if name == "." || name == "/" || name == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	prepared, err := h.Transfers.PrepareOwnedDownload(c.Request().Context(), job.ID, c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer prepared.Close()
	etag := strconv.Quote(prepared.Revision)
	c.Response().Header().Set("ETag", etag)
	if c.QueryParam("verify") == "true" {
		if c.Request().Header.Get("If-Range") != etag {
			return sftpProblem(c, sshcSFTP.ErrConflict)
		}
		if _, err := h.Transfers.BeginDownload(job.ID, prepared.Size, etag, job.TransferredBytes); err != nil {
			return sftpProblem(c, err)
		}
		if _, err := h.Transfers.VerifyDownloadComplete(job.ID, prepared.Size, etag); err != nil {
			return sftpProblem(c, err)
		}
		return c.NoContent(http.StatusNoContent)
	}
	offset, ranged, err := downloadOffset(c.Request().Header.Get("Range"), prepared.Size)
	if err != nil {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", prepared.Size))
		return problem(c, http.StatusRequestedRangeNotSatisfiable, "sftp_range_invalid")
	}
	// If-Range pins a resumed download to the revision that produced its
	// already-received prefix. If the path now names another revision, ignore
	// Range and send the new file from byte zero; the browser then discards its
	// old chunks before accepting this response.
	if ranged && c.Request().Header.Get("If-Range") != etag {
		offset, ranged = 0, false
	}
	job, err = h.Transfers.BeginDownload(job.ID, prepared.Size, etag, offset)
	if err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Response().Header().Set("Accept-Ranges", "bytes")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(prepared.Size-offset, 10))
	if ranged {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, prepared.Size-1, prepared.Size))
		c.Response().WriteHeader(http.StatusPartialContent)
	}
	output := &countingWriter{write: c.Response().Write, progress: func(written int64) error {
		_, progressErr := h.Transfers.RecordDownloadSent(job.ID, offset+written, prepared.Size, etag)
		return progressErr
	}}
	if _, err := prepared.WriteFrom(c.Request().Context(), offset, output); err != nil {
		if output.written > 0 {
			return err
		}
		return sftpProblem(c, err)
	}
	if _, err := h.Transfers.RecordDownloadSent(job.ID, offset+output.written, prepared.Size, etag); err != nil {
		return err
	}
	return nil
}

func downloadOffset(header string, size int64) (int64, bool, error) {
	if header == "" {
		return 0, false, nil
	}
	if !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, false, sshcSFTP.ErrOffsetMismatch
	}
	value := strings.TrimPrefix(header, "bytes=")
	start, end, ok := strings.Cut(value, "-")
	if !ok || start == "" || end != "" {
		return 0, false, sshcSFTP.ErrOffsetMismatch
	}
	offset, err := strconv.ParseInt(start, 10, 64)
	if err != nil || offset < 0 || offset >= size {
		return 0, false, sshcSFTP.ErrOffsetMismatch
	}
	return offset, true, nil
}

func (h SFTPHandlers) DownloadArchive(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	job, done, err := h.Transfers.StartDownloadDataPlane(c.QueryParam("jobId"), c.Param("alias"), remotePath, sshcSFTP.TransferFolder)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer done()
	if job.TransferredBytes != 0 {
		return sftpProblem(c, sshcSFTP.ErrOffsetMismatch)
	}
	name := path.Base(remotePath)
	if name == "." || name == "/" || name == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	prepared, err := h.Transfers.PrepareOwnedArchive(c.Request().Context(), job.ID, c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer prepared.Close()
	c.Response().Header().Set("Content-Type", "application/zip")
	etag := strconv.Quote(prepared.Revision)
	c.Response().Header().Set("ETag", etag)
	if _, err := h.Transfers.BeginDownload(job.ID, prepared.Size, etag, 0); err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zip"}))
	c.Response().Header().Set("Content-Length", strconv.FormatInt(prepared.Size, 10))
	output := &countingWriter{write: c.Response().Write, progress: func(written int64) error {
		_, progressErr := h.Transfers.RecordDownloadSent(job.ID, written, prepared.Size, etag)
		return progressErr
	}}
	if _, err := prepared.WriteFrom(c.Request().Context(), 0, output); err != nil {
		if output.written > 0 {
			return err
		}
		return sftpProblem(c, err)
	}
	if _, err := h.Transfers.RecordDownloadSent(job.ID, output.written, prepared.Size, etag); err != nil {
		return err
	}
	return nil
}

type countingWriter struct {
	write    func([]byte) (int, error)
	progress func(int64) error
	written  int64
}

func (writer *countingWriter) Write(contents []byte) (int, error) {
	written, err := writer.write(contents)
	writer.written += int64(written)
	if err == nil && writer.progress != nil {
		err = writer.progress(writer.written)
	}
	return written, err
}

type resumableUploadResponse struct {
	ID               string                 `json:"id"`
	Path             string                 `json:"path"`
	Offset           int64                  `json:"offset"`
	Size             int64                  `json:"size"`
	ExpectedRevision string                 `json:"expectedRevision"`
	CompletedRanges  []sshcSFTP.UploadRange `json:"completedRanges"`
	Parallelism      int                    `json:"parallelism"`
	ChunkBytes       int64                  `json:"chunkBytes"`
}

type sftpStartUploadRequest struct {
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	SourceFingerprint string `json:"sourceFingerprint"`
}

type sftpCompleteUploadRequest struct {
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	ExpectedRevision  string `json:"expectedRevision"`
	SourceFingerprint string `json:"sourceFingerprint"`
}

func describeResumableUpload(upload sshcSFTP.ResumableUpload) resumableUploadResponse {
	ranges := append([]sshcSFTP.UploadRange(nil), upload.CompletedRanges...)
	if ranges == nil {
		ranges = []sshcSFTP.UploadRange{}
	}
	return resumableUploadResponse{
		ID: upload.ID, Path: upload.Path, Offset: upload.Offset, Size: upload.Size,
		ExpectedRevision: upload.ExpectedRevision, CompletedRanges: ranges,
		Parallelism: upload.Parallelism, ChunkBytes: upload.ChunkBytes,
	}
}

func (h SFTPHandlers) StartUpload(c *echo.Context) error {
	var body sftpStartUploadRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if body.SourceFingerprint == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	upload, err := h.Transfers.StartOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), body.Path, sshcSFTP.StartUploadOptions{
		Size: body.Size, SourceFingerprint: body.SourceFingerprint,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeResumableUpload(upload))
}

func (h SFTPHandlers) AppendUpload(c *echo.Context) error {
	offset, err := strconv.ParseInt(c.QueryParam("offset"), 10, 64)
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	total, err := strconv.ParseInt(c.QueryParam("total"), 10, 64)
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if c.QueryParam("range") == "true" {
		length, parseErr := strconv.ParseInt(c.QueryParam("length"), 10, 64)
		if parseErr != nil || length <= 0 || c.Request().ContentLength > length {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		upload, rangeErr := h.Transfers.AppendRangeOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path"), offset, total, length, c.Request().Body)
		if rangeErr != nil {
			return sftpProblem(c, rangeErr)
		}
		return c.JSON(http.StatusOK, describeResumableUpload(upload))
	}
	contents, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxRequestBodyCeiling+1))
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(contents) > MaxRequestBodyCeiling {
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_transfer_too_large")
	}
	upload, err := h.Transfers.AppendOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path"), offset, total, contents)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeResumableUpload(upload))
}

func (h SFTPHandlers) CompleteUpload(c *echo.Context) error {
	var body sftpCompleteUploadRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	transfer, err := h.Transfers.CompleteOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), body.Path, body.Size, body.ExpectedRevision, body.SourceFingerprint)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, sftpTransferResponse{
		Path: transfer.Path, Bytes: transfer.Bytes, Revision: transfer.Revision,
	})
}

func (h SFTPHandlers) CancelUpload(c *echo.Context) error {
	if err := h.Transfers.CancelOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path")); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, changedResponse{Changed: true})
}

func (h SFTPHandlers) Delete(c *echo.Context) error {
	alias, remotePath := c.Param("alias"), c.QueryParam("path")
	unlock, err := h.Transfers.LockOperation(alias, remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	target := alias + ":" + remotePath
	if allowed, response := h.Actions.consume(c, session.ActionSFTPDelete, target); !allowed {
		return response
	}
	if err := h.Service.Delete(c.Request().Context(), alias, remotePath); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, changedResponse{Changed: true})
}

func (h SFTPHandlers) Chmod(c *echo.Context) error {
	var body sftpChmodRequest
	if err := decodeJSON(c, &body); err != nil || (len(body.Mode) != 3 && len(body.Mode) != 4) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	parsed, err := strconv.ParseUint(body.Mode, 8, 12)
	if err != nil || parsed > 0o777 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	target := alias + ":" + body.Path + ":" + body.Mode
	if allowed, response := h.Actions.consume(c, session.ActionSFTPChmod, target); !allowed {
		return response
	}
	entry, err := h.Service.Chmod(c.Request().Context(), alias, body.Path, fs.FileMode(parsed), body.ExpectedRevision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func addSFTPActions(registry actionRegistry, service *sshcSFTP.Service) {
	registry[session.ActionSFTPDelete] = actionKind{
		evidence: func(ctx context.Context, target string) (string, error) {
			alias, remotePath, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(ctx, alias, remotePath)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%d", entry.Type, entry.Revision, entry.Size), nil
		},
		fail: sftpProblem,
	}
	registry[session.ActionSFTPChmod] = actionKind{
		evidence: func(ctx context.Context, target string) (string, error) {
			alias, remainder, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			separator := strings.LastIndexByte(remainder, ':')
			if separator <= 0 || separator == len(remainder)-1 {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(ctx, alias, remainder[:separator])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%s", entry.Type, entry.Revision, entry.Mode.Perm()), nil
		},
		fail: sftpProblem,
	}
}

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

	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type SFTPHandlers struct {
	Service   *sshcSFTP.Service
	Transfers *sshcSFTP.TransferManager
	Actions   ActionHandlers
}

type sftpEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModifiedAt string `json:"modifiedAt"`
	Revision   string `json:"revision"`
}

func registerSFTPRoutes(engine *echo.Echo, handlers SFTPHandlers) {
	engine.GET("/api/v1/sftp/transfers", handlers.ListTransfers)
	engine.POST("/api/v1/sftp/transfers", handlers.CreateTransfer)
	engine.POST("/api/v1/sftp/transfers/:id/actions", handlers.UpdateTransfer)
	engine.GET("/api/v1/sftp/:alias/entries", handlers.List)
	engine.POST("/api/v1/sftp/:alias/entries", handlers.Mkdir)
	engine.GET("/api/v1/sftp/:alias/text", handlers.ReadText)
	engine.PUT("/api/v1/sftp/:alias/text", handlers.SaveText)
	engine.GET("/api/v1/sftp/:alias/download", handlers.Download)
	engine.GET("/api/v1/sftp/:alias/archive", handlers.DownloadArchive)
	engine.POST("/api/v1/sftp/:alias/upload", handlers.Upload)
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
		Name: entry.Name, Path: entry.Path, Type: string(entry.Type), Size: entry.Size,
		Mode: entry.Mode.String(), ModifiedAt: entry.ModifiedAt.UTC().Format(time.RFC3339Nano), Revision: entry.Revision,
	}
}

func sftpProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return problem(c, http.StatusNotFound, "sftp_not_found")
	case errors.Is(err, sshcSFTP.ErrTransferNotFound):
		return problem(c, http.StatusNotFound, "sftp_transfer_not_found")
	case errors.Is(err, sshcSFTP.ErrInvalidAlias), errors.Is(err, sshcSFTP.ErrInvalidPath), errors.Is(err, sshcSFTP.ErrRootOperation), errors.Is(err, sshcSFTP.ErrRevisionRequired), errors.Is(err, sshcSFTP.ErrInvalidTransfer):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, sshcSFTP.ErrConflict), errors.Is(err, sshcSFTP.ErrOffsetMismatch), errors.Is(err, sshcSFTP.ErrUploadIncomplete):
		return problem(c, http.StatusConflict, "sftp_conflict")
	case errors.Is(err, sshcSFTP.ErrTransferState):
		return problem(c, http.StatusConflict, "sftp_transfer_state")
	case errors.Is(err, sshcSFTP.ErrTransferLimit):
		return problem(c, http.StatusConflict, "sftp_transfer_limit")
	case errors.Is(err, sshcSFTP.ErrAlreadyExists):
		return problem(c, http.StatusConflict, "sftp_exists")
	case errors.Is(err, sshcSFTP.ErrTextTooLarge), errors.Is(err, sshcSFTP.ErrTransferTooLarge):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_text_too_large")
	case errors.Is(err, sshcSFTP.ErrNotUTF8):
		return problem(c, http.StatusUnprocessableEntity, "sftp_not_utf8")
	case errors.Is(err, sshcSFTP.ErrNotRegularFile), errors.Is(err, sshcSFTP.ErrNotDirectory):
		return problem(c, http.StatusUnprocessableEntity, "sftp_wrong_type")
	default:
		return problem(c, http.StatusBadGateway, "sftp_failed")
	}
}

type sftpTransferJobResponse struct {
	ID               string  `json:"id"`
	BatchID          string  `json:"batchId"`
	Alias            string  `json:"alias"`
	Direction        string  `json:"direction"`
	Kind             string  `json:"kind"`
	Name             string  `json:"name"`
	RemotePath       string  `json:"remotePath"`
	TotalBytes       int64   `json:"totalBytes"`
	TransferredBytes int64   `json:"transferredBytes"`
	BytesPerSecond   float64 `json:"bytesPerSecond"`
	RemainingSeconds int64   `json:"remainingSeconds"`
	Status           string  `json:"status"`
	Attempt          int     `json:"attempt"`
	Problem          string  `json:"problem"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

func describeTransferJob(job sshcSFTP.TransferJob) sftpTransferJobResponse {
	return sftpTransferJobResponse{
		ID: job.ID, BatchID: job.BatchID, Alias: job.Alias, Direction: string(job.Direction),
		Kind: string(job.Kind), Name: job.Name, RemotePath: job.RemotePath, TotalBytes: job.TotalBytes,
		TransferredBytes: job.TransferredBytes, BytesPerSecond: job.BytesPerSecond,
		RemainingSeconds: job.RemainingSeconds, Status: string(job.Status), Attempt: job.Attempt,
		Problem: job.Problem, CreatedAt: job.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: job.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func (h SFTPHandlers) ListTransfers(c *echo.Context) error {
	jobs := h.Transfers.ListJobs()
	described := make([]sftpTransferJobResponse, 0, len(jobs))
	for _, job := range jobs {
		described = append(described, describeTransferJob(job))
	}
	return c.JSON(http.StatusOK, struct {
		MaxConcurrent int                       `json:"maxConcurrent"`
		Jobs          []sftpTransferJobResponse `json:"jobs"`
	}{MaxConcurrent: h.Transfers.MaxConcurrent(), Jobs: described})
}

func (h SFTPHandlers) CreateTransfer(c *echo.Context) error {
	var body struct {
		ID         string `json:"id"`
		BatchID    string `json:"batchId"`
		Alias      string `json:"alias"`
		Direction  string `json:"direction"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		RemotePath string `json:"remotePath"`
		TotalBytes int64  `json:"totalBytes"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.CreateJob(sshcSFTP.CreateTransferJob{
		ID: body.ID, BatchID: body.BatchID, Alias: body.Alias,
		Direction: sshcSFTP.TransferDirection(body.Direction), Kind: sshcSFTP.TransferKind(body.Kind),
		Name: body.Name, RemotePath: body.RemotePath, TotalBytes: body.TotalBytes,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeTransferJob(job))
}

func (h SFTPHandlers) UpdateTransfer(c *echo.Context) error {
	var body struct {
		Action           string `json:"action"`
		TransferredBytes *int64 `json:"transferredBytes"`
		Problem          string `json:"problem"`
		ResetProgress    bool   `json:"resetProgress"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.UpdateJob(c.Param("id"), sshcSFTP.UpdateTransferJob{
		Action: sshcSFTP.TransferJobAction(body.Action), TransferredBytes: body.TransferredBytes,
		Problem: body.Problem, ResetProgress: body.ResetProgress,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeTransferJob(job))
}

func (h SFTPHandlers) List(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	entries, err := h.Service.List(c.Request().Context(), c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]sftpEntry, 0, len(entries))
	for _, entry := range entries {
		described = append(described, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, struct {
		Path    string      `json:"path"`
		Entries []sftpEntry `json:"entries"`
	}{Path: remotePath, Entries: described})
}

func (h SFTPHandlers) ReadText(c *echo.Context) error {
	file, err := h.Service.ReadText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, struct {
		Entry    sftpEntry `json:"entry"`
		Contents string    `json:"contents"`
		Revision string    `json:"revision"`
	}{describeSFTPEntry(file.Entry), file.Contents, file.Revision})
}

func (h SFTPHandlers) SaveText(c *echo.Context) error {
	var body struct {
		Contents         string `json:"contents"`
		ExpectedRevision string `json:"expectedRevision"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	file, err := h.Service.SaveText(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), body.Contents, body.ExpectedRevision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, struct {
		Entry    sftpEntry `json:"entry"`
		Contents string    `json:"contents"`
		Revision string    `json:"revision"`
	}{describeSFTPEntry(file.Entry), file.Contents, file.Revision})
}

func (h SFTPHandlers) Mkdir(c *echo.Context) error {
	var body struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := decodeJSON(c, &body); err != nil || body.Type != "directory" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	entry, err := h.Service.Mkdir(c.Request().Context(), c.Param("alias"), body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Rename(c *echo.Context) error {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	entry, err := h.Service.Rename(c.Request().Context(), c.Param("alias"), body.From, body.To)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func (h SFTPHandlers) Download(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	name := path.Base(remotePath)
	if name == "." || name == "/" || name == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	entry, err := h.Service.Stat(c.Request().Context(), c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	if entry.Type != sshcSFTP.EntryFile {
		return sftpProblem(c, sshcSFTP.ErrNotRegularFile)
	}
	offset, ranged, err := downloadOffset(c.Request().Header.Get("Range"), entry.Size)
	if err != nil {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", entry.Size))
		return problem(c, http.StatusRequestedRangeNotSatisfiable, "sftp_range_invalid")
	}
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Response().Header().Set("Accept-Ranges", "bytes")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(entry.Size-offset, 10))
	if ranged {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, entry.Size-1, entry.Size))
		c.Response().WriteHeader(http.StatusPartialContent)
	}
	output := &countingWriter{write: c.Response().Write}
	if _, err := h.Service.DownloadFrom(c.Request().Context(), c.Param("alias"), remotePath, offset, output); err != nil {
		if output.written > 0 {
			return err
		}
		return sftpProblem(c, err)
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
	name := path.Base(remotePath)
	if name == "." || name == "/" || name == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zip"}))
	output := &countingWriter{write: c.Response().Write}
	if _, err := h.Service.DownloadArchive(c.Request().Context(), c.Param("alias"), remotePath, output); err != nil {
		if output.written > 0 {
			return err
		}
		return sftpProblem(c, err)
	}
	return nil
}

type countingWriter struct {
	write   func([]byte) (int, error)
	written int64
}

func (writer *countingWriter) Write(contents []byte) (int, error) {
	written, err := writer.write(contents)
	writer.written += int64(written)
	return written, err
}

func (h SFTPHandlers) Upload(c *echo.Context) error {
	overwrite, err := strconv.ParseBool(c.QueryParam("overwrite"))
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	transfer, err := h.Service.Upload(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), c.Request().Body, sshcSFTP.UploadOptions{
		Overwrite: overwrite, MaxBytes: MaxRequestBodyCeiling,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, transfer)
}

type resumableUploadResponse struct {
	ID               string `json:"id"`
	Path             string `json:"path"`
	Offset           int64  `json:"offset"`
	Size             int64  `json:"size"`
	ExpectedRevision string `json:"expectedRevision"`
}

func describeResumableUpload(upload sshcSFTP.ResumableUpload) resumableUploadResponse {
	return resumableUploadResponse{
		ID: upload.ID, Path: upload.Path, Offset: upload.Offset, Size: upload.Size,
		ExpectedRevision: upload.ExpectedRevision,
	}
}

func (h SFTPHandlers) StartUpload(c *echo.Context) error {
	var body struct {
		Path             string `json:"path"`
		Size             int64  `json:"size"`
		Overwrite        bool   `json:"overwrite"`
		ExpectedRevision string `json:"expectedRevision"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	upload, err := h.Transfers.Start(c.Request().Context(), c.Param("alias"), c.Param("id"), body.Path, sshcSFTP.StartUploadOptions{
		Size: body.Size, Overwrite: body.Overwrite, ExpectedRevision: body.ExpectedRevision,
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
	contents, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxRequestBodyCeiling+1))
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(contents) > MaxRequestBodyCeiling {
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_transfer_too_large")
	}
	upload, err := h.Transfers.Append(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path"), offset, total, contents)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeResumableUpload(upload))
}

func (h SFTPHandlers) CompleteUpload(c *echo.Context) error {
	var body struct {
		Path             string `json:"path"`
		Size             int64  `json:"size"`
		ExpectedRevision string `json:"expectedRevision"`
	}
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	transfer, err := h.Transfers.Complete(c.Request().Context(), c.Param("alias"), c.Param("id"), body.Path, body.Size, body.ExpectedRevision)
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, struct {
		Path     string `json:"path"`
		Bytes    int64  `json:"bytes"`
		Revision string `json:"revision"`
	}{transfer.Path, transfer.Bytes, transfer.Revision})
}

func (h SFTPHandlers) CancelUpload(c *echo.Context) error {
	if err := h.Transfers.Cancel(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path")); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, map[string]bool{"changed": true})
}

func (h SFTPHandlers) Delete(c *echo.Context) error {
	alias, remotePath := c.Param("alias"), c.QueryParam("path")
	target := alias + ":" + remotePath
	if allowed, response := h.Actions.consume(c, session.ActionSFTPDelete, target); !allowed {
		return response
	}
	if err := h.Service.Delete(c.Request().Context(), alias, remotePath); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, map[string]bool{"changed": true})
}

func (h SFTPHandlers) Chmod(c *echo.Context) error {
	var body struct {
		Path             string `json:"path"`
		Mode             string `json:"mode"`
		ExpectedRevision string `json:"expectedRevision"`
	}
	if err := decodeJSON(c, &body); err != nil || (len(body.Mode) != 3 && len(body.Mode) != 4) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	parsed, err := strconv.ParseUint(body.Mode, 8, 12)
	if err != nil || parsed > 0o777 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
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
		evidence: func(target string) (string, error) {
			alias, remotePath, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(context.Background(), alias, remotePath)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%d", entry.Type, entry.Revision, entry.Size), nil
		},
		fail: sftpProblem,
	}
	registry[session.ActionSFTPChmod] = actionKind{
		evidence: func(target string) (string, error) {
			alias, remainder, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			separator := strings.LastIndexByte(remainder, ':')
			if separator <= 0 || separator == len(remainder)-1 {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(context.Background(), alias, remainder[:separator])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%s", entry.Type, entry.Revision, entry.Mode.Perm()), nil
		},
		fail: sftpProblem,
	}
}

package httpserver

import (
	"context"
	"errors"
	"fmt"
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
	Service *sshcSFTP.Service
	Actions ActionHandlers
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
	engine.GET("/api/v1/sftp/:alias/entries", handlers.List)
	engine.POST("/api/v1/sftp/:alias/entries", handlers.Mkdir)
	engine.GET("/api/v1/sftp/:alias/text", handlers.ReadText)
	engine.PUT("/api/v1/sftp/:alias/text", handlers.SaveText)
	engine.GET("/api/v1/sftp/:alias/download", handlers.Download)
	engine.GET("/api/v1/sftp/:alias/archive", handlers.DownloadArchive)
	engine.POST("/api/v1/sftp/:alias/upload", handlers.Upload)
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
	case errors.Is(err, sshcSFTP.ErrInvalidAlias), errors.Is(err, sshcSFTP.ErrInvalidPath), errors.Is(err, sshcSFTP.ErrRootOperation), errors.Is(err, sshcSFTP.ErrRevisionRequired):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, sshcSFTP.ErrConflict):
		return problem(c, http.StatusConflict, "sftp_conflict")
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
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	output := &countingWriter{write: c.Response().Write}
	if _, err := h.Service.Download(c.Request().Context(), c.Param("alias"), remotePath, output); err != nil {
		if output.written > 0 {
			return err
		}
		return sftpProblem(c, err)
	}
	return nil
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

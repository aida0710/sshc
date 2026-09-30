package httpserver

import (
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	sshcSFTP "sshc/internal/sftp"
)

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
		if err := h.Transfers.VerifyOwnedDownload(job.ID, prepared, etag); err != nil {
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
	download, err := h.Transfers.BeginOwnedDownload(sshcSFTP.OwnedDownloadRequest{
		JobID: job.ID, Prepared: prepared, Revision: etag, Offset: offset,
	})
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
	return sendDownloadBody(c, download)
}

// sendDownloadBody writes the body after the headers. Once a byte has gone
// out, the status went with it, so a later error can only end the response.
func sendDownloadBody(c *echo.Context, download *sshcSFTP.PinnedDownload) error {
	written, err := download.Send(c.Request().Context(), c.Response())
	if err != nil && written == 0 {
		return sftpProblem(c, err)
	}
	return err
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
	download, err := h.Transfers.BeginOwnedDownload(sshcSFTP.OwnedDownloadRequest{
		JobID: job.ID, Prepared: prepared, Revision: etag,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zip"}))
	c.Response().Header().Set("Content-Length", strconv.FormatInt(prepared.Size, 10))
	return sendDownloadBody(c, download)
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

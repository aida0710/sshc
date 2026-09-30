package httpserver

import (
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	sshcSFTP "sshc/internal/sftp"
)

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
		upload, rangeErr := h.Transfers.AppendRangeOwned(c.Request().Context(), sshcSFTP.UploadRangeWrite{
			Target:   sshcSFTP.UploadTarget{Alias: c.Param("alias"), ID: c.Param("id"), RemotePath: c.QueryParam("path")},
			Range:    sshcSFTP.UploadRange{Offset: offset, Size: length},
			Total:    total,
			Contents: c.Request().Body,
		})
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
	upload, err := h.Transfers.AppendOwned(c.Request().Context(), sshcSFTP.UploadAppend{
		Target: sshcSFTP.UploadTarget{Alias: c.Param("alias"), ID: c.Param("id"), RemotePath: c.QueryParam("path")},
		Offset: offset, Total: total, Contents: contents,
	})
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
	transfer, err := h.Transfers.CompleteOwned(c.Request().Context(), sshcSFTP.UploadCompletion{
		Target: sshcSFTP.UploadTarget{Alias: c.Param("alias"), ID: c.Param("id"), RemotePath: body.Path},
		Total:  body.Size, ExpectedRevision: body.ExpectedRevision, SourceFingerprint: body.SourceFingerprint,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, SFTPTransfer{
		Path: transfer.Path, Bytes: transfer.Bytes, Revision: transfer.Revision,
	})
}

func (h SFTPHandlers) CancelUpload(c *echo.Context) error {
	if err := h.Transfers.CancelOwned(c.Request().Context(), c.Param("alias"), c.Param("id"), c.QueryParam("path")); err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, changedResponse{Changed: true})
}

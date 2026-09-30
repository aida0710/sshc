package httpserver

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v5"

	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/validate"
	"sshc/internal/vpnrefusal"
)

func sftpProblem(c *echo.Context, err error) error {
	switch {
	// A spool error wraps the engine's own file error. It comes before the
	// fs errors below, which describe the remote side.
	case errors.Is(err, sshcSFTP.ErrSpoolUnavailable):
		return problem(c, http.StatusServiceUnavailable, "sftp_spool_unavailable")
	case errors.Is(err, sshcSFTP.ErrSpoolFull):
		return problem(c, http.StatusInsufficientStorage, "sftp_spool_full")
	case errors.Is(err, validate.ErrUnsafeAlias):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, sshcSFTP.ErrLinkLoop):
		return problem(c, http.StatusNotFound, "sftp_not_found")
	case errors.Is(err, fs.ErrPermission):
		return problem(c, http.StatusForbidden, "sftp_permission_denied")
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
	case errors.Is(err, sshcSFTP.ErrTraversalLimit):
		return problem(c, http.StatusRequestEntityTooLarge, "sftp_traversal_limit")
	}
	// VPN の経路を用意できなかった理由は、VPN の語のまま返す。画面は VPN 画面と
	// 同じ言い方で見せる。
	if refusal, known := vpnrefusal.Of(err); known {
		return problemWith(c, http.StatusBadGateway, problemPayload{
			Code: refusal.Code, Field: refusal.Field, Reason: refusal.Reason, Limit: refusal.Limit,
		})
	}
	return problem(c, http.StatusBadGateway, "sftp_failed")
}

package httpserver

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/remotesync"
)

// Mapping remotesync failures to the problem codes the browser wording
// keys off.

func syncProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, remotesync.ErrNotConfigured):
		return problem(c, http.StatusConflict, "sync_not_configured")
	case errors.Is(err, remotesync.ErrRemoteMoved):
		return problem(c, http.StatusConflict, "sync_remote_moved")
	case errors.Is(err, remotesync.ErrRemoteDeleted):
		return problem(c, http.StatusConflict, "sync_remote_deleted")
	case errors.Is(err, remotesync.ErrPreviewStale):
		return problem(c, http.StatusConflict, "preview_stale")
	case errors.Is(err, remotesync.ErrRecoveryRequired):
		return problem(c, http.StatusConflict, "sync_key_recovery_required")
	case errors.Is(err, remotesync.ErrRecoveryTargetChange):
		return problem(c, http.StatusConflict, "sync_key_recovery_target_change")
	case errors.Is(err, remotesync.ErrSetupTargetChanged):
		return problem(c, http.StatusConflict, "sync_setup_target_changed")
	case errors.Is(err, remotesync.ErrSetupTargetIncomplete):
		return problem(c, http.StatusConflict, "sync_setup_target_incomplete")
	case errors.Is(err, remotesync.ErrHistoryKeyLossConfirmation):
		return problem(c, http.StatusConflict, "sync_history_key_loss_confirmation_required")
	case errors.Is(err, remotesync.ErrNothingToPush):
		return problem(c, http.StatusConflict, "sync_nothing_to_push")
	case errors.Is(err, remotesync.ErrNoSnapshot):
		return problem(c, http.StatusNotFound, "sync_no_snapshot")
	case errors.Is(err, remotesync.ErrConflicts):
		return problem(c, http.StatusConflict, "sync_conflicts")
	case errors.Is(err, remotesync.ErrPushRefused):
		return problem(c, http.StatusConflict, "sync_push_refused")
	case errors.Is(err, remotesync.ErrApplyRefused):
		return problem(c, http.StatusConflict, "sync_apply_refused")
	case errors.Is(err, remotesync.ErrForcePushTarget):
		return problem(c, http.StatusBadRequest, "sync_force_target_invalid")
	case errors.Is(err, remotesync.ErrHistoryTarget):
		return problem(c, http.StatusBadRequest, "sync_history_target_invalid")
	case errors.Is(err, remotesync.ErrCommitMessage):
		return problem(c, http.StatusBadRequest, "sync_commit_message_invalid")
	case errors.Is(err, remotesync.ErrInvalidIgnoreRules):
		return problem(c, http.StatusBadRequest, "sync_ignore_invalid")
	case errors.Is(err, remotesync.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.Is(err, remotesync.ErrWeakPassphrase):
		return problem(c, http.StatusBadRequest, "passphrase_too_short")
	case errors.Is(err, remotesync.ErrCostRefused):
		return problem(c, http.StatusConflict, "snapshot_cost_refused")
	case errors.Is(err, remotesync.ErrObjectTooLarge), errors.Is(err, remotesync.ErrSnapshotTooLarge):
		return problem(c, http.StatusConflict, "snapshot_too_large")
	case errors.Is(err, remotesync.ErrUnsupportedEnvelopeVersion), errors.Is(err, remotesync.ErrUnsupportedVersion):
		return problem(c, http.StatusConflict, "snapshot_schema_unsupported")
	case errors.Is(err, remotesync.ErrUnsafePath), errors.Is(err, remotesync.ErrUnsafeMode),
		errors.Is(err, remotesync.ErrManifestMismatch), errors.Is(err, remotesync.ErrNotASnapshot):
		return problem(c, http.StatusConflict, "snapshot_rejected")
	case errors.Is(err, remotesync.ErrAuthenticationFailed):
		return problem(c, http.StatusBadGateway, "bucket_authentication_failed")
	case errors.Is(err, remotesync.ErrAccessDenied):
		return problem(c, http.StatusBadGateway, "bucket_access_denied")
	case errors.Is(err, remotesync.ErrRateLimited):
		return problem(c, http.StatusTooManyRequests, "bucket_rate_limited")
	case errors.Is(err, remotesync.ErrServiceUnavailable):
		return problem(c, http.StatusServiceUnavailable, "bucket_unavailable")
	case errors.Is(err, remotesync.ErrRefused), errors.Is(err, remotesync.ErrInsecureEndpoint):
		return problem(c, http.StatusBadGateway, "bucket_refused")
	case errors.Is(err, context.DeadlineExceeded):
		return problemDetail(c, http.StatusGatewayTimeout, "bucket_timeout", "the object store did not respond before the request timeout")
	case isSyncDNSError(err):
		return problemDetail(c, http.StatusBadGateway, "bucket_dns_failed", "the object store hostname could not be resolved")
	case isSyncTLSError(err):
		return problemDetail(c, http.StatusBadGateway, "bucket_tls_failed", "the secure connection to the object store could not be verified")
	case isSyncNetworkError(err):
		return problemDetail(c, http.StatusBadGateway, "bucket_unreachable", "the network connection to the object store could not be established")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return problemDetail(c, http.StatusBadGateway, "snapshot_download_incomplete", "the encrypted snapshot download ended before it was complete")
	case remotesync.IsLocalChange(err):
		return problem(c, http.StatusConflict, "sync_local_changed")
	case errors.Is(err, remotesync.ErrWorkspaceBusy):
		return problem(c, http.StatusConflict, "sync_workspace_busy")
	default:
		return problemDetail(c, http.StatusInternalServerError, "sync_internal_failed", "the synchronization operation failed for an unclassified internal reason")
	}
}

func isSyncDNSError(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns)
}

func isSyncTLSError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	return errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid)
}

func isSyncNetworkError(err error) bool {
	var network net.Error
	return errors.As(err, &network)
}

func autoSyncFailureProblem(c *echo.Context, detail string) error {
	switch detail {
	case "not_configured":
		return problem(c, http.StatusConflict, "sync_not_configured")
	case "wrong_passphrase":
		return problem(c, http.StatusForbidden, detail)
	case "bucket_timeout":
		return problem(c, http.StatusGatewayTimeout, detail)
	case "bucket_authentication_failed", "bucket_access_denied", "bucket_refused", "bucket_dns_failed", "bucket_tls_failed", "bucket_unreachable", "snapshot_download_incomplete":
		return problem(c, http.StatusBadGateway, detail)
	case "bucket_rate_limited":
		return problem(c, http.StatusTooManyRequests, detail)
	case "bucket_unavailable":
		return problem(c, http.StatusServiceUnavailable, detail)
	case "remote_moved", "remote_deleted", "conflicts", "snapshot_cost_refused", "snapshot_too_large",
		"snapshot_schema_unsupported", "snapshot_rejected", "sync_ignore_invalid":
		return problem(c, http.StatusConflict, detail)
	default:
		return problem(c, http.StatusInternalServerError, "sync_internal_failed")
	}
}

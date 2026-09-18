package remotesync

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
)

// FailureKind は、失敗を HTTP status や画面の扱いへ写すための大分類である。
// code（文字列）が「何が起きたか」を、kind が「どう扱うか」を表す。
type FailureKind int

const (
	// FailureInternal は分類できない内部失敗。
	FailureInternal FailureKind = iota
	// FailureConflict は現在の状態と食い違う要求。取り込み直せば解ける。
	FailureConflict
	// FailureNotFound は存在しないものへの要求。
	FailureNotFound
	// FailureInvalid は要求の内容自体が不正。
	FailureInvalid
	// FailureForbidden は鍵が合わない。
	FailureForbidden
	// FailureGateway は object store が拒否した、または届かない。
	FailureGateway
	// FailureRateLimited は object store が要求数を制限している。
	FailureRateLimited
	// FailureUnavailable は object store が一時的に応答しない。
	FailureUnavailable
	// FailureTimeout は object store が期限内に応答しなかった。
	FailureTimeout
)

// Failure は、機密情報を含みうる error を、画面用の安定した code と kind に変換した結果。
type Failure struct {
	Code string
	Kind FailureKind
}

var internalFailure = Failure{"sync_internal_failed", FailureInternal}

type failureRule struct {
	matches func(error) bool
	failure Failure
}

func is(targets ...error) func(error) bool {
	return func(err error) bool {
		for _, target := range targets {
			if errors.Is(err, target) {
				return true
			}
		}
		return false
	}
}

// failureRules は、この package のあらゆる失敗を code と kind に写す唯一の表である。
// HTTP の問題応答と自動同期の表示が同じ表を使うので、同じ失敗が場所によって別の
// 名前になることはない。順序にも意味がある: io.ErrUnexpectedEOF は net.Error に
// 包まれて届くことがあり、ネットワーク一般より先に見る。
var failureRules = []failureRule{
	{is(ErrNotConfigured), Failure{"sync_not_configured", FailureConflict}},
	{is(ErrRemoteMoved), Failure{"sync_remote_moved", FailureConflict}},
	{is(ErrRemoteDeleted), Failure{"sync_remote_deleted", FailureConflict}},
	{is(ErrPreviewStale), Failure{"preview_stale", FailureConflict}},
	{is(ErrRecoveryRequired), Failure{"sync_key_recovery_required", FailureConflict}},
	{is(ErrRecoveryTargetChange), Failure{"sync_key_recovery_target_change", FailureConflict}},
	{is(ErrSetupTargetChanged), Failure{"sync_setup_target_changed", FailureConflict}},
	{is(ErrSetupTargetIncomplete), Failure{"sync_setup_target_incomplete", FailureConflict}},
	{is(ErrHistoryKeyLossConfirmation), Failure{"sync_history_key_loss_confirmation_required", FailureConflict}},
	{is(ErrNothingToPush), Failure{"sync_nothing_to_push", FailureConflict}},
	{is(ErrNoSnapshot), Failure{"sync_no_snapshot", FailureNotFound}},
	{is(ErrConflicts), Failure{"sync_conflicts", FailureConflict}},
	{is(ErrPushRefused), Failure{"sync_push_refused", FailureConflict}},
	{is(ErrApplyRefused), Failure{"sync_apply_refused", FailureConflict}},
	{is(ErrForcePushTarget), Failure{"sync_force_target_invalid", FailureInvalid}},
	{is(ErrHistoryTarget), Failure{"sync_history_target_invalid", FailureInvalid}},
	{is(ErrCommitMessage), Failure{"sync_commit_message_invalid", FailureInvalid}},
	{is(ErrInvalidIgnoreRules), Failure{"sync_ignore_invalid", FailureInvalid}},
	{is(ErrWrongPassphrase), Failure{"wrong_passphrase", FailureForbidden}},
	{is(ErrWeakPassphrase), Failure{"passphrase_too_short", FailureInvalid}},
	{is(ErrCostRefused), Failure{"snapshot_cost_refused", FailureConflict}},
	{is(ErrObjectTooLarge, ErrSnapshotTooLarge), Failure{"snapshot_too_large", FailureConflict}},
	{is(ErrUnsupportedEnvelopeVersion, ErrUnsupportedVersion), Failure{"snapshot_schema_unsupported", FailureConflict}},
	{is(ErrUnsafePath, ErrUnsafeMode, ErrManifestMismatch, ErrNotASnapshot), Failure{"snapshot_rejected", FailureConflict}},
	{is(ErrAuthenticationFailed), Failure{"bucket_authentication_failed", FailureGateway}},
	{is(ErrAccessDenied), Failure{"bucket_access_denied", FailureGateway}},
	{is(ErrRateLimited), Failure{"bucket_rate_limited", FailureRateLimited}},
	{is(ErrServiceUnavailable), Failure{"bucket_unavailable", FailureUnavailable}},
	{is(ErrRefused, ErrInsecureEndpoint), Failure{"bucket_refused", FailureGateway}},
	{is(context.DeadlineExceeded), Failure{"bucket_timeout", FailureTimeout}},
	{isDNSError, Failure{"bucket_dns_failed", FailureGateway}},
	{isTLSError, Failure{"bucket_tls_failed", FailureGateway}},
	{is(io.ErrUnexpectedEOF), Failure{"snapshot_download_incomplete", FailureGateway}},
	{isNetworkError, Failure{"bucket_unreachable", FailureGateway}},
	{IsLocalChange, Failure{"sync_local_changed", FailureConflict}},
	{is(ErrWorkspaceBusy), Failure{"sync_workspace_busy", FailureConflict}},
}

// Classify は err を code と kind に写す。
func Classify(err error) Failure {
	for _, rule := range failureRules {
		if rule.matches(err) {
			return rule.failure
		}
	}
	return internalFailure
}

// FailureForCode は、自動同期の表示が保持した code を Failure に戻す。表に無い code は
// 内部失敗として扱う。
func FailureForCode(code string) Failure {
	for _, rule := range failureRules {
		if rule.failure.Code == code {
			return rule.failure
		}
	}
	return internalFailure
}

// FailureCode returns the secret-free stable code for err. Engine diagnostics
// pair it with a local-only error without exposing implementation text.
func FailureCode(err error) string { return Classify(err).Code }

func isDNSError(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns)
}

func isTLSError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	return errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid)
}

func isNetworkError(err error) bool {
	var network net.Error
	return errors.As(err, &network)
}

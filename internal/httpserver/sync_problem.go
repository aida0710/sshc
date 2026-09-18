package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// remotesync の失敗を HTTP の問題応答へ写す。code と大分類は remotesync.Classify が
// 決め、ここは大分類を status に変えるだけである。自動同期の表示が持つ code も
// 同じ表から出るので、同じ失敗が場所によって別の名前になることはない。

// vaultUnavailable は、Sync の操作が保管庫に届かなかった理由のうち、利用者が解錠
// （または作成）すれば解ける 2 つをまとめる。Sync の文脈ではどちらも「保管庫が
// 使えない」なので同じ vault_locked で返す。
func vaultUnavailable(err error) bool {
	return errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault)
}

func syncProblem(c *echo.Context, err error) error {
	failure := remotesync.Classify(err)
	return syncFailureProblem(c, failure)
}

// autoSyncFailureProblem は、自動同期の一巡が残した code をそのまま問題応答にする。
func autoSyncFailureProblem(c *echo.Context, code string) error {
	return syncFailureProblem(c, remotesync.FailureForCode(code))
}

func syncFailureProblem(c *echo.Context, failure remotesync.Failure) error {
	status := syncFailureStatus(failure.Kind)
	if detail, described := syncFailureDetails[failure.Code]; described {
		return problemDetail(c, status, failure.Code, detail)
	}
	return problem(c, status, failure.Code)
}

func syncFailureStatus(kind remotesync.FailureKind) int {
	switch kind {
	case remotesync.FailureConflict:
		return http.StatusConflict
	case remotesync.FailureNotFound:
		return http.StatusNotFound
	case remotesync.FailureInvalid:
		return http.StatusBadRequest
	case remotesync.FailureForbidden:
		return http.StatusForbidden
	case remotesync.FailureGateway:
		return http.StatusBadGateway
	case remotesync.FailureRateLimited:
		return http.StatusTooManyRequests
	case remotesync.FailureUnavailable:
		return http.StatusServiceUnavailable
	case remotesync.FailureTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// syncFailureDetails は、code だけでは伝わらない失敗に添える固定の説明文。
// 秘密や要求の内容は含まない。
var syncFailureDetails = map[string]string{
	"bucket_timeout":               "the object store did not respond before the request timeout",
	"bucket_dns_failed":            "the object store hostname could not be resolved",
	"bucket_tls_failed":            "the secure connection to the object store could not be verified",
	"bucket_unreachable":           "the network connection to the object store could not be established",
	"snapshot_download_incomplete": "the encrypted snapshot download ended before it was complete",
	"sync_internal_failed":         "the synchronization operation failed for an unclassified internal reason",
}

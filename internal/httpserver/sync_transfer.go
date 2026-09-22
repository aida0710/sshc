package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/session"
)

// Moving snapshots: push, force push and the preview-then-apply pull.

func (h SyncHandlers) Push(c *echo.Context) error {
	h.restore()
	var request api.SyncPushRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	message, err := remotesync.NormalizeCommitMessage(request.Message)
	if err != nil {
		return syncProblem(c, err)
	}
	result, err := h.Service.PushUsing(c.Request().Context(), h.keyProvider(), message)
	if err != nil {
		return syncKeyProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.PushResponse{
		Status: h.statusResponse(),
		Result: pushResultResponse(result),
	})
}

// pushResultResponse は、この push が記録した変更をパスだけで返す。null ではなく
// 空配列を返すのは、応答の形を呼び出し側の分岐なしで読めるようにするためである。
func pushResultResponse(result remotesync.PushResult) api.PushResult {
	return api.PushResult{
		Summary: snapshotSummaryResponse(result.Summary), ObjectCount: result.ObjectCount,
		UploadedBytes: result.UploadedBytes, CompletedAt: result.CompletedAt,
		Added:    append([]string{}, result.Added...),
		Modified: append([]string{}, result.Modified...),
		Removed:  append([]string{}, result.Removed...),
	}
}

func (h SyncHandlers) PushDraft(c *echo.Context) error {
	h.restore()
	draft, err := h.Service.PushDraft()
	if err != nil {
		return syncProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SyncPushDraft{
		Message: draft.Message, Added: draft.Added, Modified: draft.Modified, Removed: draft.Removed,
	})
}

func (h SyncHandlers) ForcePush(c *echo.Context) error {
	h.restore()
	var request api.SyncPushRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	message, err := remotesync.NormalizeCommitMessage(request.Message)
	if err != nil {
		return syncProblem(c, err)
	}
	confirmation, err := h.Service.ForcePushConfirmation(c.Request().Context(), remotesync.ForcePushTarget)
	if err != nil {
		return syncProblem(c, err)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSyncForcePush,
		remotesync.ForcePushTarget, confirmation.Evidence); !allowed {
		return response
	}
	result, err := h.Service.ForcePushUsing(c.Request().Context(), h.keyProvider(), confirmation, message)
	if err != nil {
		return syncKeyProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.PushResponse{
		Status: h.statusResponse(),
		Result: pushResultResponse(result),
	})
}

// Pull は既定でプレビューし、求められたときだけ適用する。
//
// 応答が運ぶのはパスだけで、中身は決して運ばない。ファイルのバイト列を保持する pull
// の応答は、response body に秘密鍵を置くことになる。ユーザーが承認する
// ファイル単位のプレビューは、この application がすでに読めるファイルから組み立てられる。
func (h SyncHandlers) Pull(c *echo.Context) error {
	h.restore()
	var request api.PullRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Apply != nil && *request.Apply &&
		(request.ExpectedETag == nil || request.ExpectedRevision == nil) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if (request.HistoryKey != nil && (len(*request.HistoryKey) == 0 || len(*request.HistoryKey) > 1024)) ||
		(request.ExpectedETag != nil && len(*request.ExpectedETag) > 1024) ||
		(request.ExpectedRevision != nil && !validSyncRevision(*request.ExpectedRevision)) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	resolve := remotesync.ResolveNone
	if request.Resolve != nil {
		switch *request.Resolve {
		case api.Local:
			resolve = remotesync.ResolveLocal
		case api.Remote:
			resolve = remotesync.ResolveRemote
		default:
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
	}
	historyKey := ""
	if request.HistoryKey != nil {
		historyKey = *request.HistoryKey
	}
	acceptRemoteHead := request.AcceptRemoteHead != nil && *request.AcceptRemoteHead
	if acceptRemoteHead && (h.Service.Direction() == remotesync.DirectionPush ||
		historyKey != "" || resolve != remotesync.ResolveRemote) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var result remotesync.PullResult
	var err error
	if request.Apply != nil && *request.Apply {
		if acceptRemoteHead {
			result, err = h.Service.PullAndApplyRemoteHeadUsing(c.Request().Context(), h.keyProvider(),
				*request.ExpectedETag, *request.ExpectedRevision)
		} else {
			result, err = h.Service.PullAndApplyUsing(c.Request().Context(), h.keyProvider(), resolve, historyKey,
				*request.ExpectedETag, *request.ExpectedRevision)
		}
	} else {
		key, ok, keyErr := h.sealingKey(c)
		if !ok {
			return keyErr
		}
		if acceptRemoteHead {
			result, err = h.Service.PullRemoteHead(c.Request().Context(), key)
		} else if historyKey == "" {
			result, err = h.Service.Pull(c.Request().Context(), key, resolve)
		} else {
			result, err = h.Service.PullHistory(c.Request().Context(), key, historyKey, resolve)
		}
	}
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		return syncKeyProblem(c, err)
	}

	response := api.PullResponse{
		Applied: false, Summary: snapshotSummaryResponse(result.Summary),
		DownloadedBytes: result.DownloadedBytes, CompletedAt: result.CompletedAt,
		Conflicts:  make([]api.SyncConflict, 0, len(result.Conflicts)),
		Written:    append([]string{}, result.Written...),
		Added:      append([]string{}, result.Added...),
		Removed:    append([]string{}, result.Removed...),
		RemoteETag: result.ETag, RemoteRevision: result.Manifest.Revision,
	}
	for _, conflict := range result.Conflicts {
		response.Conflicts = append(response.Conflicts, syncConflictResponse(conflict))
	}
	if result.Origin != "" {
		origin := result.Origin
		response.Origin = &origin
	}

	if request.Apply != nil && *request.Apply {
		response.Applied = true
		if h.Auto != nil {
			h.Auto.ManualApplyCompleted()
		}
		// Apply is a second request and therefore a second download. Its
		// completion is the point after the workspace transaction and state
		// write, not the earlier moment when this request finished downloading.
		state := h.Service.SyncState()
		if state.LastOperation != nil && state.LastOperation.Kind == remotesync.OperationApply {
			response.CompletedAt = state.LastOperation.CompletedAt
		}
	}
	return c.JSON(http.StatusOK, response)
}

func syncConflictResponse(conflict remotesync.Conflict) api.SyncConflict {
	response := api.SyncConflict{
		Path: conflict.Path,
		ChangedHere: (conflict.LocalDigest != "" || conflict.LocalMode != "") &&
			(conflict.LocalDigest != conflict.BaseDigest || conflict.LocalMode != conflict.BaseMode),
		ChangedThere: conflict.RemoteDigest != conflict.BaseDigest || conflict.RemoteMode != conflict.BaseMode,
	}
	if conflict.BaseMode != "" {
		response.BaseMode = &conflict.BaseMode
	}
	if conflict.LocalMode != "" {
		response.LocalMode = &conflict.LocalMode
	}
	if conflict.RemoteMode != "" {
		response.RemoteMode = &conflict.RemoteMode
	}
	return response
}

func validSyncRevision(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

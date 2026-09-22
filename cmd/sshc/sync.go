package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/remotesync"
	"sshc/internal/session"
)

type commandFailure struct {
	Kind      string `json:"kind"`
	Retryable bool   `json:"retryable"`
}

type commandEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Success       bool            `json:"success"`
	Status        *api.SyncStatus `json:"status,omitempty"`
	Result        any             `json:"result,omitempty"`
	Failure       *commandFailure `json:"failure,omitempty"`
}

var errSyncPullRequiresForce = errors.New("sync pull requires --force")

// pullRefusal は、適用せずに止めたプレビューを持ったままの拒否である。
// 人向けの表示は、--force を案内する前に、このプレビューを見せる。
type pullRefusal struct{ preview api.PullResponse }

func (refusal pullRefusal) Error() string { return errSyncPullRequiresForce.Error() }

func (refusal pullRefusal) Unwrap() error { return errSyncPullRequiresForce }

func runSync(ctx context.Context, called syncInvocation, environment commandEnvironment) int {
	stateDir, client, stdin, stdout, stderr, terminal :=
		environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	var setupPrompt *os.File
	if called.Action == syncSetup {
		var err error
		setupPrompt, err = requireSyncSetupTerminal(stdin, stderr, terminal)
		if err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	}
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	switch called.Action {
	case syncSetup:
		if err := runSyncSetup(ctx, engine, stdin, stdout, setupPrompt, terminal); err != nil {
			return finishSyncFailure(false, err, stdout, stderr)
		}
		return 0
	case syncPush:
		result, err := runSyncPush(ctx, engine, called.Force)
		if err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Result: result,
			}); err != nil {
				return 1
			}
		} else {
			writeSyncPushResult(stdout, result)
		}
		return 0
	case syncPull:
		result, err := runSyncPull(ctx, engine, called.Force)
		if err != nil {
			var refusal pullRefusal
			if !called.JSON && errors.As(err, &refusal) {
				writeSyncChanges(stderr, pullChanges(refusal.preview))
			}
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Result: result,
			}); err != nil {
				return 1
			}
		} else {
			writeSyncPullResult(stdout, result)
		}
		return 0
	case syncNow, syncAuto:
		var status api.SyncStatus
		var err error
		if called.Action == syncNow {
			err = engine.sendJSON(ctx, http.MethodPost, "/api/v1/sync/now", struct{}{}, &status)
		} else {
			err = engine.sendJSON(ctx, http.MethodPut, "/api/v1/sync/auto",
				api.AutoSyncRequest{Enabled: called.Enabled}, &status)
		}
		if err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Result: status,
			}); err != nil {
				return 1
			}
		} else {
			writeSyncStatus(stdout, status)
		}
		return 0
	case syncStatus:
		var status api.SyncStatus
		if err := engine.getJSON(ctx, "/api/v1/sync", &status); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Status: &status,
			}); err != nil {
				return 1
			}
			return 0
		}
		writeSyncStatus(stdout, status)
		return 0
	default:
		return finishSyncFailure(called.JSON, errors.New("sync action is not implemented"), stdout, stderr)
	}
}

func runSyncPull(ctx context.Context, engine *engineAPI, force bool) (api.PullResponse, error) {
	apply := false
	previewRequest := api.PullRequest{Apply: &apply}
	if force {
		resolve := api.Remote
		previewRequest.Resolve = &resolve
		accept := true
		previewRequest.AcceptRemoteHead = &accept
	}
	var preview api.PullResponse
	if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/sync/pull", previewRequest, &preview); err != nil {
		return api.PullResponse{}, err
	}
	if !validPullResponseShape(preview) || preview.Applied {
		return api.PullResponse{}, errEngineInvalidResponse
	}
	if !force && (len(preview.Conflicts) != 0 || len(preview.Removed) != 0) {
		return api.PullResponse{}, pullRefusal{preview: preview}
	}
	if preview.RemoteETag == "" || len(preview.RemoteRevision) != 64 {
		return api.PullResponse{}, errEngineInvalidResponse
	}
	apply = true
	applyRequest := api.PullRequest{
		Apply: &apply, ExpectedETag: &preview.RemoteETag, ExpectedRevision: &preview.RemoteRevision,
	}
	if force {
		resolve := api.Remote
		applyRequest.Resolve = &resolve
		accept := true
		applyRequest.AcceptRemoteHead = &accept
	}
	var applied api.PullResponse
	if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/sync/pull", applyRequest, &applied); err != nil {
		return api.PullResponse{}, err
	}
	if !validPullResponseShape(applied) || !applied.Applied {
		return api.PullResponse{}, errEngineInvalidResponse
	}
	return applied, nil
}

func validPullResponseShape(response api.PullResponse) bool {
	return response.DownloadedBytes >= 0 && response.Summary.FileCount >= 0 &&
		response.Summary.SourceBytes >= 0 && response.Summary.SnapshotBytes >= 0
}

func runSyncPush(ctx context.Context, engine *engineAPI, force bool) (api.PushResponse, error) {
	var draft api.SyncPushDraft
	if err := engine.getJSON(ctx, "/api/v1/sync/push", &draft); err != nil {
		return api.PushResponse{}, err
	}
	if strings.TrimSpace(draft.Message) == "" || draft.Added < 0 || draft.Modified < 0 || draft.Removed < 0 {
		return api.PushResponse{}, errEngineInvalidResponse
	}
	request := api.SyncPushRequest{Message: draft.Message}
	var response api.PushResponse
	if force {
		action, err := engine.issueAction(ctx, session.ActionSyncForcePush, remotesync.ForcePushTarget)
		if err != nil {
			return api.PushResponse{}, err
		}
		if err := engine.sendJSONWithAction(ctx, http.MethodPost, "/api/v1/sync/force-push",
			action.Token, request, &response); err != nil {
			return api.PushResponse{}, err
		}
	} else if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/sync/push", request, &response); err != nil {
		return api.PushResponse{}, err
	}
	if response.Result.CompletedAt == "" || response.Result.ObjectCount < 0 ||
		response.Result.UploadedBytes < 0 || response.Result.Summary.FileCount < 0 ||
		response.Result.Summary.SourceBytes < 0 || response.Result.Summary.SnapshotBytes < 0 {
		return api.PushResponse{}, errEngineInvalidResponse
	}
	return response, nil
}

func writeCommandEnvelope(out io.Writer, envelope commandEnvelope) error {
	return json.NewEncoder(out).Encode(envelope)
}

func finishSyncFailure(asJSON bool, err error, stdout, stderr io.Writer) int {
	failure := classifyCommandFailure(err)
	exit := 1
	if errors.Is(err, context.Canceled) {
		exit = 130
	}
	if asJSON {
		_ = writeCommandEnvelope(stdout, commandEnvelope{
			SchemaVersion: 1, Success: false, Failure: &failure,
		})
		return exit
	}
	writeHumanSyncFailure(stderr, failure)
	return exit
}

func classifyCommandFailure(err error) commandFailure {
	var problem engineProblem
	if errors.As(err, &problem) && problem.OutcomeUnknown {
		return commandFailure{Kind: "outcome_unknown", Retryable: false}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return commandFailure{Kind: "canceled", Retryable: true}
	case errors.Is(err, fs.ErrNotExist):
		return commandFailure{Kind: "engine_not_running", Retryable: true}
	case errors.Is(err, handoff.ErrSchemaVersion), errors.Is(err, handoff.ErrProtocolVersion):
		return commandFailure{Kind: "engine_incompatible", Retryable: false}
	case errors.Is(err, errEngineIdentityMismatch):
		return commandFailure{Kind: "engine_mismatch", Retryable: false}
	case errors.Is(err, errEngineVaultMissing):
		return commandFailure{Kind: "vault_missing", Retryable: false}
	case errors.Is(err, errEngineVaultLocked):
		return commandFailure{Kind: "vault_locked", Retryable: false}
	case errors.Is(err, errSyncSetupTTY):
		return commandFailure{Kind: "interactive_terminal_required", Retryable: false}
	case errors.Is(err, errSyncSetupInput):
		return commandFailure{Kind: "invalid_setup_input", Retryable: false}
	case errors.Is(err, errSyncSetupIncomplete):
		return commandFailure{Kind: "sync_setup_target_incomplete", Retryable: false}
	case errors.Is(err, errSyncPullRequiresForce):
		return commandFailure{Kind: "sync_pull_requires_force", Retryable: false}
	case errors.Is(err, errEngineInvalidResponse), errors.Is(err, errEngineResponseTooLarge):
		return commandFailure{Kind: "invalid_engine_response", Retryable: false}
	}
	if errors.As(err, &problem) {
		retryable := problem.Retryable
		switch problem.Code {
		case "sync_remote_moved", "sync_remote_deleted", "preview_stale", "sync_setup_target_changed":
			retryable = true
		case "bucket_authentication_failed", "bucket_access_denied":
			retryable = false
		}
		return commandFailure{Kind: problem.Code, Retryable: retryable}
	}
	return commandFailure{Kind: "engine_unavailable", Retryable: true}
}

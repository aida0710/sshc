package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
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

func runSync(
	ctx context.Context,
	called syncInvocation,
	stateDir string,
	client *http.Client,
	stdin *os.File,
	stdout, stderr io.Writer,
	terminal passwordTerminal,
) int {
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
		return api.PullResponse{}, errSyncPullRequiresForce
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
	return response.Conflicts != nil && response.Written != nil && response.Removed != nil &&
		response.DownloadedBytes >= 0 && response.Summary.FileCount >= 0 &&
		response.Summary.SourceBytes >= 0 && response.Summary.SnapshotBytes >= 0
}

func writeSyncPullResult(out io.Writer, response api.PullResponse) {
	if !response.Applied && len(response.Written) == 0 && len(response.Removed) == 0 && len(response.Conflicts) == 0 {
		fmt.Fprintln(out, "sync pull: no changes")
		return
	}
	result := "previewed"
	if response.Applied {
		result = "applied"
	}
	rows := [][2]string{
		{"result", result},
		{"completed", dash(response.CompletedAt)},
		{"files", strconv.Itoa(response.Summary.FileCount)},
		{"written", strconv.Itoa(len(response.Written))},
		{"removed", strconv.Itoa(len(response.Removed))},
		{"conflicts", strconv.Itoa(len(response.Conflicts))},
		{"downloaded bytes", bytesValue(response.DownloadedBytes)},
		{"source bytes", bytesValue(response.Summary.SourceBytes)},
		{"snapshot bytes", bytesValue(response.Summary.SnapshotBytes)},
	}
	writeSyncRows(out, rows)
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

func writeSyncPushResult(out io.Writer, response api.PushResponse) {
	rows := [][2]string{
		{"result", "pushed"},
		{"completed", response.Result.CompletedAt},
		{"files", strconv.Itoa(response.Result.Summary.FileCount)},
		{"objects", strconv.Itoa(response.Result.ObjectCount)},
		{"source bytes", bytesValue(response.Result.Summary.SourceBytes)},
		{"snapshot bytes", bytesValue(response.Result.Summary.SnapshotBytes)},
		{"uploaded bytes", bytesValue(response.Result.UploadedBytes)},
	}
	writeSyncRows(out, rows)
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

func writeHumanSyncFailure(stderr io.Writer, failure commandFailure) {
	switch failure.Kind {
	case "canceled":
		fmt.Fprintln(stderr, "sshc: sync was canceled")
	case "engine_not_running":
		fmt.Fprintln(stderr, "sshc: no engine is running; start the desktop app or run sshc engine in another terminal")
	case "engine_incompatible", "engine_mismatch":
		fmt.Fprintln(stderr, "sshc: the CLI and running engine are incompatible; update whichever is older and restart it")
	case "vault_missing":
		fmt.Fprintln(stderr, "sshc: no vault exists; run sshc vault create")
	case "vault_locked":
		fmt.Fprintln(stderr, "sshc: the vault is locked; run sshc vault unlock")
	case "interactive_terminal_required":
		fmt.Fprintln(stderr, "sshc: sync setup requires interactive terminal input and prompt output")
	case "invalid_setup_input":
		fmt.Fprintln(stderr, "sshc: sync setup input is invalid; check the endpoint, bucket, path, region, direction, and credential lengths")
	case "sync_setup_target_incomplete":
		fmt.Fprintln(stderr, "sshc: the sync target contains an incomplete snapshot; inspect or repair the target before setup")
	case "sync_pull_requires_force":
		fmt.Fprintln(stderr, "sshc: pull includes conflicts or removals; inspect the preview and rerun with sshc sync pull --force to accept remote state")
	case "sync_not_configured":
		fmt.Fprintln(stderr, "sshc: sync is not configured; run sshc sync setup")
	case "sync_remote_moved", "sync_remote_deleted", "preview_stale", "sync_setup_target_changed":
		fmt.Fprintf(stderr, "sshc: remote sync state changed (%s); run the command again to inspect the new state\n", failure.Kind)
	case "bucket_authentication_failed":
		fmt.Fprintln(stderr, "sshc: the object store could not authenticate the request; check the access key and secret")
	case "bucket_access_denied":
		fmt.Fprintln(stderr, "sshc: the object store denied access; check the credentials, bucket, region, and key permissions")
	case "bucket_rate_limited":
		fmt.Fprintln(stderr, "sshc: the object store is rate limiting requests; wait and try again")
	case "bucket_unavailable":
		fmt.Fprintln(stderr, "sshc: the object store service is temporarily unavailable; try again later")
	case "outcome_unknown":
		fmt.Fprintln(stderr, "sshc: the sync operation outcome is unknown; do not rerun it until you inspect sshc sync status and the remote target")
	case "transport_error", "sync_failed", "bucket_refused", "engine_unavailable":
		fmt.Fprintln(stderr, "sshc: the engine or sync target is unavailable; check the engine and network, then try again")
	case "invalid_engine_response", "response_too_large", "http_error":
		fmt.Fprintln(stderr, "sshc: the running engine returned an invalid response; check that the CLI and engine versions match")
	default:
		fmt.Fprintf(stderr, "sshc: sync failed (%s); inspect sshc sync status before trying again\n", failure.Kind)
	}
}

func writeSyncStatus(out io.Writer, status api.SyncStatus) {
	rows := [][2]string{
		{"configured", yesNo(status.Configured)},
		{"vault", lockedState(status.Locked)},
		{"sync key", configuredState(status.KeyConfigured)},
		{"endpoint", dash(status.Endpoint)},
		{"bucket", dash(status.Bucket)},
		{"path", optionalString(status.Path)},
		{"region", optionalString(status.Region)},
		{"direction", dash(string(status.Direction))},
		{"auto", enabledState(status.Auto.Enabled)},
		{"auto phase", dash(string(status.Auto.Phase))},
		{"auto at", optionalString(status.Auto.At)},
		{"auto detail", optionalString(status.Auto.Detail)},
		{"last synced", optionalString(status.LastSyncedAt)},
		{"origin", optionalString(status.Origin)},
		{"files", optionalInt(status.FileCount)},
	}
	if operation := status.LastOperation; operation != nil {
		rows = append(rows,
			[2]string{"last operation", dash(string(operation.Kind))},
			[2]string{"operation completed", dash(operation.CompletedAt)},
			[2]string{"operation files", strconv.Itoa(operation.Summary.FileCount)},
			[2]string{"source bytes", bytesValue(operation.Summary.SourceBytes)},
			[2]string{"snapshot bytes", bytesValue(operation.Summary.SnapshotBytes)},
			[2]string{"objects", optionalInt(operation.ObjectCount)},
			[2]string{"uploaded bytes", optionalInt64(operation.UploadedBytes)},
			[2]string{"downloaded bytes", optionalInt64(operation.DownloadedBytes)},
			[2]string{"written", optionalInt(operation.Written)},
			[2]string{"removed", optionalInt(operation.Removed)},
		)
	} else {
		rows = append(rows, [2]string{"last operation", "-"})
	}
	writeSyncRows(out, rows)
}

func writeSyncRows(out io.Writer, rows [][2]string) {
	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	for _, row := range rows {
		fmt.Fprintf(out, "%-*s  %s\n", width, safeTerminalCell(row[0]), safeTerminalCell(row[1]))
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func lockedState(locked bool) string {
	if locked {
		return "locked"
	}
	return "unlocked"
}

func configuredState(configured bool) string {
	if configured {
		return "configured"
	}
	return "missing"
}

func enabledState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func optionalString(value *string) string {
	if value == nil {
		return "-"
	}
	return dash(*value)
}

func optionalInt(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

func optionalInt64(value *int64) string {
	if value == nil {
		return "-"
	}
	return bytesValue(*value)
}

func bytesValue(value int64) string { return strconv.FormatInt(value, 10) + " B" }

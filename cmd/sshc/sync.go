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

	"sshc/internal/api"
	"sshc/internal/handoff"
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
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	switch called.Action {
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
	case errors.Is(err, errEngineInvalidResponse), errors.Is(err, errEngineResponseTooLarge):
		return commandFailure{Kind: "invalid_engine_response", Retryable: false}
	}
	var problem engineProblem
	if errors.As(err, &problem) {
		retryable := problem.Retryable
		switch problem.Code {
		case "sync_remote_moved", "sync_remote_deleted", "preview_stale", "sync_setup_target_changed":
			retryable = true
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
	case "sync_not_configured":
		fmt.Fprintln(stderr, "sshc: sync is not configured; run sshc sync setup")
	case "sync_remote_moved", "sync_remote_deleted", "preview_stale", "sync_setup_target_changed":
		fmt.Fprintf(stderr, "sshc: remote sync state changed (%s); run the command again to inspect the new state\n", failure.Kind)
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
	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	for _, row := range rows {
		fmt.Fprintf(out, "%-*s  %s\n", width, row[0], row[1])
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

package main

import (
	"fmt"
	"io"
	"strconv"

	"sshc/internal/api"
)

// sshc sync の結果と失敗を、人向けの行として書く。--json の envelope は sync.go が組み立てる。

// syncChange は、一度の pull または push が動かすファイル一つと、その区分。
type syncChange struct {
	kind syncChangeKind
	path string
}

// syncChangeKind は、表示にそのまま使う区分の名前でもある。
type syncChangeKind string

const (
	changeAdded    syncChangeKind = "added"
	changeModified syncChangeKind = "modified"
	changeRemoved  syncChangeKind = "removed"
	changeConflict syncChangeKind = "conflict"
)

func writeSyncPullResult(out io.Writer, response api.PullResponse) {
	if !response.Applied && len(response.Written) == 0 && len(response.Removed) == 0 && len(response.Conflicts) == 0 {
		fmt.Fprintln(out, "sync pull: no changes")
		return
	}
	result := "previewed"
	if response.Applied {
		result = "applied"
	}
	changes := pullChanges(response)
	counts := syncChangeCounts(changes)
	rows := [][2]string{
		{"result", result},
		{"completed", dash(response.CompletedAt)},
		{"files", strconv.Itoa(response.Summary.FileCount)},
		{"added", strconv.Itoa(counts[changeAdded])},
		{"modified", strconv.Itoa(counts[changeModified])},
		{"removed", strconv.Itoa(counts[changeRemoved])},
		{"conflicts", strconv.Itoa(counts[changeConflict])},
		{"downloaded bytes", bytesValue(response.DownloadedBytes)},
		{"source bytes", bytesValue(response.Summary.SourceBytes)},
		{"snapshot bytes", bytesValue(response.Summary.SnapshotBytes)},
	}
	writeSyncResult(out, rows, changes)
}

// pullChanges は、pull の応答を 1 ファイル 1 行の区分へ直す。応答は書き込むパスを
// written にまとめ、そのうち、まだこのワークスペースに無いものだけを added で示す。
func pullChanges(response api.PullResponse) []syncChange {
	added := make(map[string]bool, len(response.Added))
	for _, path := range response.Added {
		added[path] = true
	}
	changes := make([]syncChange, 0, len(response.Written)+len(response.Removed)+len(response.Conflicts))
	for _, path := range response.Written {
		if added[path] {
			changes = append(changes, syncChange{kind: changeAdded, path: path})
		}
	}
	for _, path := range response.Written {
		if !added[path] {
			changes = append(changes, syncChange{kind: changeModified, path: path})
		}
	}
	for _, path := range response.Removed {
		changes = append(changes, syncChange{kind: changeRemoved, path: path})
	}
	for _, conflict := range response.Conflicts {
		changes = append(changes, syncChange{kind: changeConflict, path: conflict.Path})
	}
	return changes
}

func writeSyncPushResult(out io.Writer, response api.PushResponse) {
	changes := pushChanges(response.Result)
	counts := syncChangeCounts(changes)
	rows := [][2]string{
		{"result", "pushed"},
		{"completed", response.Result.CompletedAt},
		{"files", strconv.Itoa(response.Result.Summary.FileCount)},
		{"added", strconv.Itoa(counts[changeAdded])},
		{"modified", strconv.Itoa(counts[changeModified])},
		{"removed", strconv.Itoa(counts[changeRemoved])},
		{"objects", strconv.Itoa(response.Result.ObjectCount)},
		{"source bytes", bytesValue(response.Result.Summary.SourceBytes)},
		{"snapshot bytes", bytesValue(response.Result.Summary.SnapshotBytes)},
		{"uploaded bytes", bytesValue(response.Result.UploadedBytes)},
	}
	writeSyncResult(out, rows, changes)
}

// pushChanges は、この push が親スナップショットに対して記録した変更を並べる。
func pushChanges(result api.PushResult) []syncChange {
	changes := make([]syncChange, 0, len(result.Added)+len(result.Modified)+len(result.Removed))
	for _, path := range result.Added {
		changes = append(changes, syncChange{kind: changeAdded, path: path})
	}
	for _, path := range result.Modified {
		changes = append(changes, syncChange{kind: changeModified, path: path})
	}
	for _, path := range result.Removed {
		changes = append(changes, syncChange{kind: changeRemoved, path: path})
	}
	return changes
}

// writeSyncResult は、要約の表を書き、変更したファイルがあれば一行空けて続ける。
func writeSyncResult(out io.Writer, rows [][2]string, changes []syncChange) {
	writeSyncRows(out, rows)
	if len(changes) == 0 {
		return
	}
	fmt.Fprintln(out)
	writeSyncChanges(out, changes)
}

// writeSyncChanges は、動かしたファイルを 1 行ずつ、区分と揃えて書く。
func writeSyncChanges(out io.Writer, changes []syncChange) {
	rows := make([][2]string, 0, len(changes))
	for _, change := range changes {
		rows = append(rows, [2]string{string(change.kind), change.path})
	}
	writeSyncRows(out, rows)
}

func syncChangeCounts(changes []syncChange) map[syncChangeKind]int {
	counts := make(map[syncChangeKind]int, 4)
	for _, change := range changes {
		counts[change.kind]++
	}
	return counts
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
		fmt.Fprintln(stderr, "sshc: pull includes conflicts or removals; nothing above was applied; rerun with sshc sync pull --force to accept remote state")
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
		{"access key", accessKeyStatus(status)},
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

func accessKeyStatus(status api.SyncStatus) string {
	if status.AccessKeySuffix != nil && *status.AccessKeySuffix != "" {
		return maskedAccessKeySuffix(*status.AccessKeySuffix)
	}
	if status.Configured {
		return "configured"
	}
	return "missing"
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

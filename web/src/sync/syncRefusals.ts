import type { MessageKey } from "../i18n/messages";

// Engine failure codes that have their own wording in the sync screen.
// Every code that internal/remotesync can return (its failure table and
// TargetRefusals) must be here; a Go test there reads this file to check it.
export const syncRefusals: Record<string, MessageKey> = {
  sync_not_configured: "sync.notConfigured",
  wrong_passphrase: "sync.wrongKey",
  sync_key_missing: "sync.keyMissing",
  passphrase_too_short: "sync.keyTooShort",
  bucket_authentication_failed: "sync.bucketAuthenticationFailed",
  bucket_access_denied: "sync.bucketAccessDenied",
  bucket_rate_limited: "sync.bucketRateLimited",
  bucket_unavailable: "sync.bucketUnavailable",
  bucket_refused: "sync.unreachable",
  bucket_timeout: "sync.bucketTimeout",
  bucket_dns_failed: "sync.bucketDNSFailed",
  bucket_tls_failed: "sync.bucketTLSFailed",
  bucket_unreachable: "sync.bucketUnreachable",
  sync_internal_failed: "sync.internalFailed",
  snapshot_download_incomplete: "sync.snapshotDownloadIncomplete",
  snapshot_cost_refused: "sync.snapshotCostRefused",
  snapshot_schema_unsupported: "sync.snapshotSchemaUnsupported",
  snapshot_rejected: "sync.snapshotRejected",
  snapshot_too_large: "sync.snapshotTooLarge",
  sync_no_snapshot: "sync.noSnapshot",
  sync_conflicts: "sync.pullConflicts",
  sync_push_refused: "sync.pushRefused",
  sync_apply_refused: "sync.applyRefused",
  sync_force_target_invalid: "sync.forcePushTargetInvalid",
  sync_history_target_invalid: "sync.historyTargetInvalid",
  endpoint_must_be_https: "sync.endpointNotHTTPS",
  endpoint_must_have_no_path: "sync.endpointPath",
  unsafe_bucket_name: "sync.bucketNameInvalid",
  unsafe_object_path: "sync.objectPathInvalid",
  sync_remote_moved: "sync.remoteMoved",
  sync_remote_deleted: "sync.remoteDeleted",
  sync_key_recovery_required: "sync.keyRecoveryRequired",
  sync_key_recovery_target_change: "sync.keyRecoveryTargetChange",
  sync_history_key_loss_confirmation_required: "sync.keyHistoryLossConfirm",
  preview_stale: "sync.previewStale",
  sync_nothing_to_push: "sync.noLocalChanges",
  sync_commit_message_invalid: "sync.commitMessageInvalid",
  sync_ignore_invalid: "sync.exclusions.invalid",
  sync_setup_target_changed: "sync.setup.changed",
  sync_setup_target_incomplete: "sync.setup.incomplete",
  sync_local_changed: "sync.localChanged",
  sync_workspace_busy: "sync.workspaceBusy",
  sync_pending_transaction: "sync.pendingTransaction",
  sync_local_path_unportable: "sync.localPathUnportable",
};

// Failures whose problem names the local file at fault. The path is relative
// to ~/.ssh and fills {path}. Without a path (the automatic sync status keeps
// only the code) the wording in syncRefusals is used.
export const syncPathRefusals: Record<string, MessageKey> = {
  sync_local_path_unportable: "sync.localPathUnportableAt",
};

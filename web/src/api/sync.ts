import { apiClient } from "./client";
import { issueAction, postEmpty, postJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type SyncStatus = components["schemas"]["SyncStatus"];
export type SyncKeyResponse = components["schemas"]["SyncKeyResponse"];
export type SyncSetupCheckRequest = components["schemas"]["SyncSetupCheckRequest"];
export type SyncSetupCheckResponse = components["schemas"]["SyncSetupCheckResponse"];
export type SyncSetupRequest = components["schemas"]["SyncSetupRequest"];
export type SyncSetupResponse = components["schemas"]["SyncSetupResponse"];
export type SyncDirection = components["schemas"]["SyncDirection"];
export type SnapshotSummary = components["schemas"]["SnapshotSummary"];
export type SyncOperation = components["schemas"]["SyncOperation"];
export type PushResult = components["schemas"]["PushResult"];
export type PushResponse = components["schemas"]["PushResponse"];
export type SyncPushDraft = components["schemas"]["SyncPushDraft"];
export type SyncExclusions = components["schemas"]["SyncExclusions"];
export type PullResponse = components["schemas"]["PullResponse"];
export type SyncBucketStatus = components["schemas"]["SyncBucketStatus"];
export type SyncHistory = components["schemas"]["SyncHistory"];
export type SyncHistoryRevision = components["schemas"]["SyncHistoryRevision"];
export type SyncHistoryDiff = components["schemas"]["SyncHistoryDiff"];

const locallyExplainedSyncFailures = [
  "bucket_authentication_failed",
  "bucket_access_denied",
  "bucket_rate_limited",
  "bucket_unavailable",
  "bucket_refused",
  "bucket_timeout",
  "bucket_dns_failed",
  "bucket_tls_failed",
  "bucket_unreachable",
  "snapshot_download_incomplete",
  "snapshot_cost_refused",
  "snapshot_schema_unsupported",
  "snapshot_rejected",
  "snapshot_too_large",
  "wrong_passphrase",
  "sync_ignore_invalid",
] as const;

export const SYNC_FORCE_PUSH_ACTION_KIND = "sync.force_push";
export const SYNC_FORCE_PUSH_TARGET = "remote-workspace";

export type SyncApi = {
  syncStatus(): Promise<SyncStatus>;
  checkSyncSetup(
    settings: SyncSetupCheckRequest,
  ): Promise<SyncSetupCheckResponse>;
  completeSyncSetup(settings: SyncSetupRequest): Promise<SyncSetupResponse>;
  syncExclusions(): Promise<SyncExclusions>;
  saveSyncExclusions(document: string): Promise<SyncExclusions>;
  syncPushDraft(): Promise<SyncPushDraft>;
  pushSnapshot(message: string): Promise<PushResponse>;
  forcePushSnapshot(message: string): Promise<PushResponse>;
  syncBucketStatus(): Promise<SyncBucketStatus>;
  syncHistory(): Promise<SyncHistory>;
  diffSyncHistory(key: string): Promise<SyncHistoryDiff>;
  pullSnapshot(
    apply: boolean,
    resolve?: "local" | "remote",
    historyKey?: string,
    expected?: Pick<PullResponse, "remoteETag" | "remoteRevision">,
    acceptRemoteHead?: boolean,
  ): Promise<PullResponse>;
  setSyncKey(
    key?: string,
    confirmHistoryLoss?: boolean,
  ): Promise<SyncKeyResponse>;
  setAutoSync(enabled: boolean): Promise<SyncStatus>;
  syncNow(): Promise<SyncStatus>;

};

function validateSyncStatus(value: unknown): SyncStatus {
  return validateOpenAPISchema<SyncStatus>("SyncStatus", value);
}

function validateSyncSetupCheck(value: unknown): SyncSetupCheckResponse {
  return validateOpenAPISchema<SyncSetupCheckResponse>("SyncSetupCheckResponse", value);
}

function validateSyncSetup(value: unknown): SyncSetupResponse {
  return validateOpenAPISchema<SyncSetupResponse>("SyncSetupResponse", value);
}

function validateSyncKey(value: unknown): SyncKeyResponse {
  return validateOpenAPISchema<SyncKeyResponse>("SyncKeyResponse", value);
}

function validatePushResponse(value: unknown): PushResponse {
  return validateOpenAPISchema<PushResponse>("PushResponse", value);
}

function validateSyncPushDraft(value: unknown): SyncPushDraft {
  return validateOpenAPISchema<SyncPushDraft>("SyncPushDraft", value);
}

function validateSyncExclusions(value: unknown): SyncExclusions {
  return validateOpenAPISchema<SyncExclusions>("SyncExclusions", value);
}

function validateSyncBucketStatus(value: unknown): SyncBucketStatus {
  return validateOpenAPISchema<SyncBucketStatus>("SyncBucketStatus", value);
}

function validateSyncHistory(value: unknown): SyncHistory {
  return validateOpenAPISchema<SyncHistory>("SyncHistory", value);
}

function validateSyncHistoryDiff(value: unknown): SyncHistoryDiff {
  return validateOpenAPISchema<SyncHistoryDiff>("SyncHistoryDiff", value);
}

function validatePullResponse(value: unknown): PullResponse {
  return validateOpenAPISchema<PullResponse>("PullResponse", value);
}

// Remote synchronization of the workspace through an object store: setup,
// status, push and pull, the history, and the shared key.
export const syncApi: SyncApi = {
  async syncStatus() {
    return validateSyncStatus(await apiClient.read("/api/v1/sync"));
  },
  async checkSyncSetup(settings) {
    return validateSyncSetupCheck(
      await postJSON<unknown>("/api/v1/sync/setup/check", settings, undefined, locallyExplainedSyncFailures),
    );
  },
  async completeSyncSetup(settings) {
    return validateSyncSetup(
      await putJSON<unknown>("/api/v1/sync/setup", settings, locallyExplainedSyncFailures),
    );
  },
  async syncExclusions() {
    return validateSyncExclusions(
      await apiClient.read("/api/v1/sync/exclusions"),
    );
  },
  async saveSyncExclusions(document) {
    return validateSyncExclusions(
      await putJSON<unknown>("/api/v1/sync/exclusions", { document }, ["sync_ignore_invalid"]),
    );
  },
  async syncPushDraft() {
    return validateSyncPushDraft(await apiClient.read("/api/v1/sync/push"));
  },
  async pushSnapshot(message) {
    return validatePushResponse(
      await postJSON<unknown>("/api/v1/sync/push", { message }),
    );
  },
  async forcePushSnapshot(message) {
    const token = await issueAction(
      SYNC_FORCE_PUSH_ACTION_KIND,
      SYNC_FORCE_PUSH_TARGET,
    );
    return validatePushResponse(
      await postJSON<unknown>("/api/v1/sync/force-push", { message }, token),
    );
  },
  async syncBucketStatus() {
    return validateSyncBucketStatus(
      await apiClient.read("/api/v1/sync/bucket"),
    );
  },
  async syncHistory() {
    return validateSyncHistory(await apiClient.read("/api/v1/sync/history"));
  },
  async diffSyncHistory(key) {
    return validateSyncHistoryDiff(
      await postJSON<unknown>("/api/v1/sync/history/diff", { key }),
    );
  },
  async pullSnapshot(apply, resolve, historyKey, expected, acceptRemoteHead) {
    if (apply && expected === undefined) throw new Error("invalid_request");
    const request = {
      apply,
      ...(resolve === undefined ? {} : { resolve }),
      ...(historyKey === undefined ? {} : { historyKey }),
      ...(acceptRemoteHead === undefined ? {} : { acceptRemoteHead }),
      ...(expected === undefined
        ? {}
        : {
            expectedETag: expected.remoteETag,
            expectedRevision: expected.remoteRevision,
          }),
    };
    return validatePullResponse(
      await postJSON<unknown>("/api/v1/sync/pull", request, undefined, [
        ...locallyExplainedSyncFailures,
        "sync_local_changed",
        "sync_workspace_busy",
      ]),
    );
  },
  async setSyncKey(key, confirmHistoryLoss) {
    return validateSyncKey(
      await putJSON<unknown>("/api/v1/sync/key", {
        ...(key === undefined ? {} : { key }),
        ...(confirmHistoryLoss === undefined ? {} : { confirmHistoryLoss }),
      }),
    );
  },
  async setAutoSync(enabled) {
    return validateSyncStatus(
      await putJSON<unknown>("/api/v1/sync/auto", { enabled }),
    );
  },
  async syncNow() {
    return validateSyncStatus(await postEmpty<unknown>("/api/v1/sync/now"));
  },
};

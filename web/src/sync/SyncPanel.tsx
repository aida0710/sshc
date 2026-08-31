import { useCallback, useEffect, useState, type ReactNode } from "react";
import {
  integrationsApi,
  type IntegrationsApi,
  type SyncDirection,
  type SyncHistoryDiff,
  type SyncStatus,
} from "../api/integrations";
import { useLanguage } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { formatBytes, SyncResultCard } from "./SyncResultCard";
import { SyncExclusionsPanel } from "./SyncExclusionsPanel";
import { useSyncSetupForm } from "./useSyncSetupForm";
import { useSyncRemoteState } from "./useSyncRemoteState";
import { useSyncOperation } from "./useSyncOperation";
import { useSyncPullPreview } from "./useSyncPullPreview";
import { SyncForcePushDialog } from "./SyncForcePushDialog";
import { SyncPullPreviewDialog } from "./SyncPullPreviewDialog";
import { SyncHistorySection } from "./SyncHistorySection";

type SyncPanelProps = { api?: IntegrationsApi };

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

function SyncRow({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <div className="border-t border-hairline first:border-t-0">
      <label className="flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2">
        <span className="w-full shrink-0 text-sm text-ink-muted sm:w-32">
          {label}
        </span>
        <span className="flex min-w-0 flex-1 justify-start sm:ml-auto sm:justify-end">
          {children}
        </span>
      </label>
      {hint === undefined ? null : (
        <p className={`px-3 pb-3 sm:pb-2 ${hintText}`}>{hint}</p>
      )}
    </div>
  );
}

function SyncErrorNotice({ message, code }: { message: string; code: string }) {
  if (message === "") return null;
  return (
    <Notice tone="danger">
      <span className="flex min-w-0 flex-col gap-1">
        <span>{message}</span>
        {code === "" ? null : (
          <code className="text-xs text-ink-muted">Code: {code}</code>
        )}
      </span>
    </Notice>
  );
}

type SyncStatusState =
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncStatus };

const refusals: Record<string, MessageKey> = {
  not_configured: "sync.notConfigured",
  wrong_master_password: "sync.wrongMaster",
  wrong_passphrase: "sync.wrongKey",
  sync_key_missing: "sync.keyMissing",
  passphrase_too_short: "sync.keyTooShort",
  bucket_refused: "sync.unreachable",
  bucket_timeout: "sync.bucketTimeout",
  bucket_dns_failed: "sync.bucketDNSFailed",
  bucket_tls_failed: "sync.bucketTLSFailed",
  bucket_unreachable: "sync.bucketUnreachable",
  sync_failed: "sync.failed",
  sync_internal_failed: "sync.internalFailed",
  snapshot_download_incomplete: "sync.snapshotDownloadIncomplete",
  snapshot_cost_refused: "sync.snapshotCostRefused",
  snapshot_schema_unsupported: "sync.snapshotSchemaUnsupported",
  snapshot_rejected: "sync.snapshotRejected",
  snapshot_too_large: "sync.snapshotTooLarge",
  sync_no_snapshot: "sync.noSnapshot",
  endpoint_must_have_no_path: "sync.endpointPath",
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
};

export function SyncPanel({ api = integrationsApi }: SyncPanelProps) {
  const { locale, t } = useLanguage();
  const [statusState, setStatusState] = useState<SyncStatusState>({
    phase: "loading",
  });
  const {
    endpoint,
    bucket,
    path,
    region,
    accessKeyId,
    secretAccessKey,
    direction,
    setupCheck,
    ownKey,
    chooseOwn,
    confirmHistoryLoss,
    editingSettings,
    settingsOpen,
    setupInput,
    editSettings,
    setEndpoint,
    setBucket,
    setPath,
    setRegion,
    setAccessKeyId,
    setSecretAccessKey,
    setDirection,
    setSetupCheck,
    setOwnKey,
    setChooseOwn,
    setConfirmHistoryLoss,
    setEditingSettings,
    setSettingsOpen,
  } = useSyncSetupForm();
  const [master, setMaster] = useState("");
  const [revealed, setRevealed] = useState("");
  const {
    bucketState,
    historyState,
    selectedHistoryKey,
    bucketHistoryExpanded,
    pushDraft,
    pushMessage,
    refreshBucket,
    refreshHistory,
    refreshPushDraft,
    resetBucket,
    resetHistory,
    resetPush,
    selectHistory: selectHistoryKey,
    toggleBucketHistory,
    editPushMessage,
    acceptPushMessage,
  } = useSyncRemoteState(api, t);
  const [historyDiff, setHistoryDiff] = useState<SyncHistoryDiff | null>(null);
  const [forcePushOpen, setForcePushOpen] = useState(false);
  const operation = useSyncOperation(t, refusals);
  const {
    resultView,
    notice,
    error,
    errorCode,
    busy,
    execute,
    clearError,
    showNotice: setNotice,
    showResult: setResultView,
  } = operation;
  const pull = useSyncPullPreview();
  const {
    preview,
    historyKey: previewHistoryKey,
    acceptRemoteHead: previewAcceptRemoteHead,
    acceptedRemovals,
    resolve,
  } = pull;
  const reload = useCallback(async () => {
    setStatusState({ phase: "loading" });
    try {
      const next = await api.syncStatus();
      setStatusState({ phase: "ready", value: next });
      if (next.configured) void refreshBucket();
      else resetBucket();
      if (next.configured && next.keyConfigured) void refreshHistory();
      else resetHistory();
      if (next.configured && next.direction !== "pull") {
        void refreshPushDraft();
      } else {
        resetPush();
      }
      if (
        next.direction === "pull" &&
        next.auto.phase === "blocked" &&
        next.auto.detail === "remote_moved"
      ) {
        clearError();
      }
    } catch {
      setStatusState({ phase: "error", message: t("sync.statusFailed") });
    }
  }, [
    api,
    refreshBucket,
    refreshHistory,
    refreshPushDraft,
    resetBucket,
    resetHistory,
    resetPush,
    t,
    clearError,
  ]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const shouldPollBucket =
    statusState.phase === "ready" &&
    statusState.value.configured &&
    !statusState.value.locked;
  useEffect(() => {
    if (!shouldPollBucket) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "hidden") void refreshBucket();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [refreshBucket, shouldPollBucket]);

  async function run<T>(
    operation: () => Promise<T>,
    apply: (value: T) => void,
    failure: string,
    explain?: (code: string) => string,
  ) {
    await execute(operation, apply, failure, explain, async (code) => {
      if (
        code !== "sync_remote_moved" ||
        statusState.phase !== "ready" ||
        statusState.value.direction !== "pull"
      ) {
        return false;
      }
      await reload();
      return true;
    });
  }

  async function previewWith(choice?: "local" | "remote", historyKey?: string) {
    pull.prepare(choice, historyKey);
    await run(
      () => api.pullSnapshot(false, choice, historyKey),
      (next) => {
        pull.show(next);
        setResultView({ kind: "preview", result: next });
        setNotice(
          next.written.length + next.removed.length + next.conflicts.length ===
            0
            ? t("sync.alreadyMatches")
            : "",
        );
      },
      t("sync.pullFailed"),
    );
  }

  async function previewCurrentRemoteHead() {
    pull.prepareRemoteHead();
    await run(
      () => api.pullSnapshot(false, "remote", undefined, undefined, true),
      (next) => {
        pull.show(next);
        setResultView({ kind: "preview", result: next });
        setNotice(
          next.written.length + next.removed.length === 0
            ? t("sync.alreadyMatches")
            : "",
        );
      },
      t("sync.pullFailed"),
    );
  }

  async function selectHistory(key: string) {
    selectHistoryKey(key);
    setHistoryDiff(null);
    await run(
      () => api.diffSyncHistory(key),
      (next) => setHistoryDiff(next),
      t("sync.historyDiffFailed"),
    );
  }

  if (statusState.phase === "loading") {
    return (
      <p role="status" className={hintText}>
        {t("sync.loading")}
      </p>
    );
  }

  if (statusState.phase === "error") {
    return (
      <div
        className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
      >
        <PageHeader
          title={t("sync.heading")}
          description={t("sync.pageDescription")}
        />
        <Notice tone="danger">{statusState.message}</Notice>
        <Button onClick={() => void reload()} className="self-start">
          {t("shell.bootstrapRetry")}
        </Button>
      </div>
    );
  }

  const status = statusState.value;

  if (status.locked) {
    return (
      <div
        className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
      >
        <PageHeader
          title={t("sync.heading")}
          description={t("sync.pageDescription")}
        />
        <SyncErrorNotice message={error} code={errorCode} />
        <section className="sshc-card grid overflow-hidden rounded-md bg-card md:grid-cols-[minmax(0,0.9fr)_minmax(18rem,1.1fr)]">
          <div className="flex flex-col justify-between gap-8 bg-toolbar p-6 md:p-8">
            <span className="flex h-12 w-12 items-center justify-center rounded-md bg-select-fill text-accent">
              <Icon name="sync" className="h-6 w-6" />
            </span>
            <div>
              <h3 className="text-lg font-semibold text-ink">
                {t("sync.bucketHeading")}
              </h3>
              <p className="mt-2 text-sm leading-6 text-ink-muted">
                {t("sync.sealed")}
              </p>
            </div>
          </div>
          <div className="flex flex-col justify-center gap-4 p-6 md:p-8">
            <Field label={t("secrets.master")}>
              <input
                type="password"
                value={master}
                onChange={(event) => setMaster(event.target.value)}
                className={control}
              />
            </Field>
            <Button
              kind="primary"
              disabled={busy || master === ""}
              onClick={() =>
                void run(
                  () => api.unlockVault(master),
                  () => {
                    setMaster("");
                    void reload();
                  },
                  t("sync.unlockFailed"),
                  (code) => (code === "vault_missing" ? t("sync.noVault") : ""),
                )
              }
              className="self-start"
            >
              {t("secrets.unlock")}
            </Button>
          </div>
        </section>
      </div>
    );
  }

  const remoteHeadBlocked =
    status.auto.phase === "blocked" && status.auto.detail === "remote_moved";
  const pushChangeCount =
    pushDraft === null
      ? null
      : pushDraft.added + pushDraft.modified + pushDraft.removed;
  const bucketHistoryItems =
    bucketState.phase === "ready" ? bucketState.value.history : [];
  const visibleBucketHistory = bucketHistoryExpanded
    ? bucketHistoryItems
    : bucketHistoryItems.slice(0, 5);
  const transferPanel =
    status.configured && status.direction !== "pull" ? (
      <section
        aria-labelledby="sync-transfer-heading"
        className="sshc-card flex flex-col gap-3 rounded-md bg-card p-4"
      >
        <h3 id="sync-transfer-heading" className={sectionHeading}>
          {t("sync.transferHeading")}
        </h3>
        <p className="text-sm leading-6 text-ink-muted">
          {t(`sync.transferHint.${status.direction}` as MessageKey)}
        </p>
        <div className="flex flex-col gap-1 border-t border-line pt-3 text-sm text-ink">
          <label htmlFor="sync-commit-message" className="font-medium">
            {t("sync.commitMessage")}
          </label>
          <input
            id="sync-commit-message"
            aria-describedby="sync-commit-message-hint"
            value={pushMessage}
            maxLength={240}
            onChange={(event) => {
              editPushMessage(event.target.value);
            }}
            className={control}
            placeholder={t("sync.commitMessagePlaceholder")}
          />
          <span id="sync-commit-message-hint" className={hintText}>
            {pushDraft === null
              ? t("sync.commitMessageHint")
              : pushChangeCount === 0
                ? t("sync.noLocalChanges")
                : t("sync.commitMessageChanges", {
                    added: pushDraft.added,
                    modified: pushDraft.modified,
                    removed: pushDraft.removed,
                  })}
          </span>
        </div>
        <div className="flex flex-wrap gap-2 border-t border-line pt-3">
          <Button
            kind="primary"
            disabled={
              busy ||
              !status.keyConfigured ||
              pushMessage.trim() === "" ||
              pushChangeCount === null ||
              pushChangeCount === 0
            }
            onClick={() =>
              void run(
                () => api.pushSnapshot(pushMessage.trim()),
                (next) => {
                  setStatusState({ phase: "ready", value: next.status });
                  pull.close();
                  setResultView({ kind: "push", result: next.result });
                  setNotice(t("sync.pushed"));
                  acceptPushMessage();
                  void refreshPushDraft();
                  void refreshBucket();
                  void refreshHistory();
                },
                t("sync.pushFailed"),
              )
            }
          >
            {t("sync.push")}
          </Button>
          <Button
            disabled={busy || !status.keyConfigured}
            onClick={() => void previewWith(undefined)}
          >
            {t("sync.preview")}
          </Button>
        </div>
      </section>
    ) : null;

  return (
    <div
      className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
    >
      <PageHeader
        title={t("sync.heading")}
        description={t("sync.pageDescription")}
      />

      {status.configured ? null : (
        <ol
          aria-label={t("sync.flowHeading")}
          className="grid overflow-hidden rounded-md border border-line bg-toolbar sm:grid-cols-3"
        >
          {["sync.flowBucket", "sync.flowKey", "sync.flowOperate"].map(
            (key, index) => (
              <li
                key={key}
                className="flex items-center gap-3 border-b border-hairline px-4 py-3 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0"
              >
                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-select-fill font-mono text-xs font-semibold text-accent">
                  {index + 1}
                </span>
                <span className="text-sm text-ink">{t(key as MessageKey)}</span>
              </li>
            ),
          )}
        </ol>
      )}

      <SyncErrorNotice message={error} code={errorCode} />
      {notice === "" ? null : (
        <p role="status" className="text-sm text-ink-muted">
          {notice}
        </p>
      )}
      {revealed === "" ? null : (
        <Notice tone="notice">
          <span className="block">{t("sync.keyShownOnce")}</span>
          <output className="mt-2 block select-all break-all font-mono text-sm">
            {revealed}
          </output>
        </Notice>
      )}

      {status.configured ? (
        <section className="sshc-card overflow-hidden rounded-md bg-card">
          <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
            <div>
              <h3 className={sectionHeading}>{t("sync.overviewHeading")}</h3>
              <p className={`mt-1 ${hintText}`}>
                {status.synced
                  ? t("sync.lastSynced", {
                      at: status.lastSyncedAt ?? "",
                      count: status.fileCount ?? 0,
                    })
                  : t("sync.neverSynced")}
              </p>
            </div>
            <span className="rounded-full bg-select-fill px-2 py-1 text-xs font-medium text-accent">
              {t(`sync.direction.${status.direction}`)}
            </span>
          </header>
          <div className="flex flex-col gap-4 p-4">
            <label className="flex items-center gap-2 text-sm text-ink">
              <input
                type="checkbox"
                checked={status.auto.enabled}
                disabled={busy || !status.keyConfigured}
                onChange={(event) =>
                  void run(
                    () => api.setAutoSync(event.target.checked),
                    (next) => setStatusState({ phase: "ready", value: next }),
                    t("sync.autoFailed"),
                  )
                }
              />
              {t("sync.autoEnable")}
            </label>
            <p className={hintText}>
              {t(`sync.autoHint.${status.direction}` as MessageKey)}
            </p>
            {remoteHeadBlocked ? (
              <div className="flex flex-col gap-3">
                <Notice tone="danger">
                  {t(
                    status.direction === "pull"
                      ? "sync.autoBlockedRemoteMovedPull"
                      : "sync.autoBlockedRemoteMoved",
                  )}
                </Notice>
                <p className={hintText}>{t("sync.remoteHeadReviewHint")}</p>
              </div>
            ) : status.auto.phase === "failed" ? (
              <SyncErrorNotice
                message={t(
                  status.auto.detail === "wrong_passphrase"
                    ? "sync.autoFailedWrongKey"
                    : status.auto.detail === "snapshot_schema_unsupported"
                      ? "sync.autoFailedSchema"
                      : (refusals[status.auto.detail ?? ""] ??
                        "sync.autoFailedLast"),
                )}
                code={status.auto.detail ?? "sync_failed"}
              />
            ) : (
              <p role="status" className={hintText}>
                {status.auto.phase === "blocked"
                  ? t(
                      status.auto.detail === "conflicts"
                        ? "sync.autoBlockedConflicts"
                        : status.auto.detail === "remote_deleted"
                          ? "sync.autoBlockedRemoteDeleted"
                          : "sync.autoBlockedRemovals",
                    )
                  : status.auto.at === undefined
                    ? t("sync.autoIdle")
                    : t("sync.autoLastRan", { at: status.auto.at })}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                kind="primary"
                disabled={busy || !status.keyConfigured}
                onClick={() =>
                  void (remoteHeadBlocked
                    ? previewCurrentRemoteHead()
                    : run(
                        () => api.syncNow(),
                        (next) =>
                          setStatusState({ phase: "ready", value: next }),
                        t("sync.autoNowFailed"),
                      ))
                }
              >
                {t(
                  remoteHeadBlocked
                    ? "sync.remoteHeadReview"
                    : (`sync.autoNow.${status.direction}` as MessageKey),
                )}
              </Button>
              {status.direction === "push" || remoteHeadBlocked ? null : (
                <Button
                  disabled={busy || !status.keyConfigured}
                  onClick={() => void previewWith(undefined)}
                >
                  {t("sync.checkRemoteChanges")}
                </Button>
              )}
              {status.direction === "push" ? null : (
                <Button
                  disabled={busy || !status.keyConfigured}
                  onClick={() => void previewCurrentRemoteHead()}
                >
                  {t("sync.forcePull")}
                </Button>
              )}
              {status.direction === "pull" ? null : (
                <Button
                  kind="danger"
                  disabled={busy || !status.keyConfigured}
                  onClick={() => setForcePushOpen(true)}
                >
                  {t("sync.forcePushShort")}
                </Button>
              )}
            </div>
          </div>
        </section>
      ) : null}

      {status.configured ? (
        <SyncExclusionsPanel
          api={api}
          onSaved={() => {
            void refreshPushDraft();
          }}
        />
      ) : null}

      <details
        open={!status.configured || settingsOpen}
        onToggle={(event) => {
          if (status.configured) setSettingsOpen(event.currentTarget.open);
        }}
        className={
          status.configured
            ? "group overflow-hidden rounded-md border border-control-line bg-card"
            : "group"
        }
      >
        {status.configured ? (
          <summary className="flex cursor-pointer list-none items-center gap-3 bg-toolbar px-4 py-3 text-sm font-medium text-ink marker:hidden hover:bg-select-fill">
            <span
              aria-hidden="true"
              className="inline-flex size-5 shrink-0 items-center justify-center text-base text-ink-muted transition-transform group-open:rotate-90"
            >
              ›
            </span>
            <span>{t("sync.manageSettings")}</span>
          </summary>
        ) : null}
        <div
          className={
            status.configured
              ? "flex flex-col gap-4 border-t border-line bg-surface-subtle p-4"
              : "flex flex-col gap-6"
          }
        >
          <p className={`rounded-md bg-toolbar px-4 py-3 ${hintText}`}>
            {t("sync.warning")}
          </p>
          <section className="overflow-hidden rounded-md border border-line bg-card">
            <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
              <div className="flex items-center gap-2">
                <Icon name="remoteKeys" className="h-4 w-4 text-ink-muted" />
                <h3 className={sectionHeading}>{t("sync.bucketHeading")}</h3>
              </div>
              {status.configured ? (
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <p className="font-mono text-xs text-ink-muted">
                    {[status.endpoint, status.bucket, status.path]
                      .filter((part) => part !== "" && part !== undefined)
                      .join("/")}

                    {status.region !== undefined && status.region !== ""
                      ? ` (${status.region})`
                      : ""}
                  </p>
                  {!editingSettings ? (
                    <Button onClick={() => editSettings(status)}>
                      {t("sync.editSettings")}
                    </Button>
                  ) : null}
                </div>
              ) : (
                <p className="text-xs text-ink-muted">
                  {t("sync.notConfigured")}
                </p>
              )}
            </header>

            {!status.configured || editingSettings ? (
              <>
                <div className="px-1 py-2 sm:px-3">
                  <SyncRow
                    label={t("sync.endpoint")}
                    hint={t("sync.endpointHint")}
                  >
                    <input
                      value={endpoint}
                      onChange={(event) => setEndpoint(event.target.value)}
                      placeholder="https://<account>.r2.cloudflarestorage.com"
                      className={control}
                    />
                  </SyncRow>
                  <SyncRow label={t("sync.bucket")}>
                    <input
                      value={bucket}
                      onChange={(event) => setBucket(event.target.value)}
                      className={control}
                    />
                  </SyncRow>

                  <SyncRow label={t("sync.path")} hint={t("sync.pathHint")}>
                    <input
                      value={path}
                      onChange={(event) => setPath(event.target.value)}
                      className={control}
                    />
                  </SyncRow>

                  <SyncRow label={t("sync.region")} hint={t("sync.regionHint")}>
                    <input
                      value={region}
                      onChange={(event) => setRegion(event.target.value)}
                      className={control}
                    />
                  </SyncRow>
                  <SyncRow label={t("sync.accessKeyId")}>
                    <input
                      value={accessKeyId}
                      onChange={(event) => setAccessKeyId(event.target.value)}
                      className={control}
                    />
                  </SyncRow>

                  <SyncRow
                    label={t("sync.secretAccessKey")}
                    hint={t("sync.credentialsNote")}
                  >
                    <input
                      type="password"
                      value={secretAccessKey}
                      onChange={(event) =>
                        setSecretAccessKey(event.target.value)
                      }
                      className={control}
                    />
                  </SyncRow>

                  <SyncRow
                    label={t("sync.direction")}
                    hint={t(`sync.direction.${direction}.hint`)}
                  >
                    <select
                      value={direction}
                      onChange={(event) =>
                        setDirection(event.target.value as SyncDirection)
                      }
                      className={control}
                    >
                      <option value="both">{t("sync.role.main")}</option>
                      <option value="pull">{t("sync.role.receive")}</option>
                      <optgroup label={t("sync.role.advanced")}>
                        <option value="push">{t("sync.role.send")}</option>
                      </optgroup>
                    </select>
                  </SyncRow>
                </div>
                <div className="flex flex-col gap-3 border-t border-line bg-toolbar px-4 py-3">
                  {setupCheck === null ? null : (
                    <Notice
                      tone={
                        setupCheck.state === "incomplete" ? "danger" : "notice"
                      }
                    >
                      {t(`sync.setup.${setupCheck.state}`)}
                      {setupCheck.state === "incomplete"
                        ? ` ${t("sync.setup.useAnotherPath")}`
                        : ""}
                    </Notice>
                  )}
                  {setupCheck === null ||
                  setupCheck.state === "incomplete" ? null : (
                    <div className="flex flex-col gap-3 rounded-lg border border-line bg-surface p-3">
                      <p className="text-sm text-ink">
                        {setupCheck.state === "existing"
                          ? t("sync.setup.existingKey")
                          : t("sync.setup.emptyKey")}
                      </p>
                      {setupCheck.state === "empty" ? (
                        <label className="flex items-center gap-2 text-sm text-ink-muted">
                          <input
                            type="checkbox"
                            checked={chooseOwn}
                            onChange={(event) =>
                              setChooseOwn(event.target.checked)
                            }
                          />
                          {t("sync.keyChooseOwn")}
                        </label>
                      ) : null}
                      {setupCheck.state === "existing" || chooseOwn ? (
                        <input
                          type="password"
                          aria-label={t("sync.keyOwnValue")}
                          value={ownKey}
                          onChange={(event) => setOwnKey(event.target.value)}
                          className={control}
                        />
                      ) : null}
                      <Button
                        kind="primary"
                        disabled={
                          busy ||
                          (setupCheck.state === "existing" && ownKey === "") ||
                          (chooseOwn && ownKey === "")
                        }
                        onClick={() =>
                          void run(
                            () =>
                              api.completeSyncSetup({
                                ...setupInput,
                                direction,
                                expectedState: setupCheck.state,
                                ...(setupCheck.etag === undefined
                                  ? {}
                                  : { expectedETag: setupCheck.etag }),
                                historyPresent: setupCheck.historyPresent,
                                key:
                                  setupCheck.state === "existing" || chooseOwn
                                    ? ownKey
                                    : "",
                              }),
                            (next) => {
                              setStatusState({
                                phase: "ready",
                                value: next.status,
                              });
                              setRevealed(next.generatedKey ?? "");
                              setOwnKey("");
                              setAccessKeyId("");
                              setSecretAccessKey("");
                              setSetupCheck(null);
                              setEditingSettings(false);
                              setSettingsOpen(false);
                              setNotice(
                                next.generatedKey === undefined
                                  ? t("sync.setup.saved")
                                  : t("sync.keyShownOnce"),
                              );
                            },
                            t("sync.configureFailed"),
                          )
                        }
                      >
                        {t("sync.setup.save")}
                      </Button>
                    </div>
                  )}
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={
                        busy ||
                        endpoint === "" ||
                        bucket === "" ||
                        accessKeyId === "" ||
                        secretAccessKey === ""
                      }
                      onClick={() =>
                        void run(
                          () => api.checkSyncSetup(setupInput),
                          (next) => {
                            setSetupCheck(next);
                            setOwnKey("");
                            setChooseOwn(false);
                          },
                          t("sync.configureFailed"),
                        )
                      }
                    >
                      {t("sync.setup.check")}
                    </Button>
                    {status.configured ? (
                      <Button
                        disabled={busy}
                        onClick={() => setEditingSettings(false)}
                      >
                        {t("sync.cancelSettings")}
                      </Button>
                    ) : null}
                  </div>
                </div>
              </>
            ) : null}
          </section>

          {status.configured ? (
            <section
              aria-labelledby="sync-key-heading"
              className="overflow-hidden rounded-md border border-line bg-card"
            >
              <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
                <div className="flex items-center gap-2">
                  <Icon name="sync" className="h-4 w-4 text-ink-muted" />
                  <h3 id="sync-key-heading" className={sectionHeading}>
                    {t("sync.key")}
                  </h3>
                </div>
                <span
                  className={`rounded-full px-2 py-1 text-xs font-medium ${status.keyConfigured ? "bg-select-fill text-accent" : "bg-notice text-notice-ink"}`}
                >
                  {status.keyConfigured
                    ? t("sync.keyReady")
                    : t("sync.keyNeeded")}
                </span>
              </header>
              <div className="flex flex-col gap-3 p-4">
                <p className="text-sm leading-6 text-ink-muted">
                  {t("sync.keyHint")}
                </p>
                <p className="text-sm font-medium text-ink">
                  {t(status.keyConfigured ? "sync.keySet" : "sync.keyMissing")}
                </p>
                <div className="flex flex-col gap-3 border-t border-line pt-3">
                  <label className="flex items-center gap-2 text-sm text-ink-muted">
                    <input
                      type="checkbox"
                      checked={chooseOwn}
                      onChange={(event) => setChooseOwn(event.target.checked)}
                    />
                    {t("sync.keyChooseOwn")}
                  </label>
                  {chooseOwn ? (
                    <input
                      type="password"
                      aria-label={t("sync.keyOwnValue")}
                      value={ownKey}
                      onChange={(event) => setOwnKey(event.target.value)}
                      className={control}
                    />
                  ) : null}
                  {status.keyConfigured ? (
                    <label className="flex items-start gap-2 text-sm text-danger">
                      <input
                        type="checkbox"
                        checked={confirmHistoryLoss}
                        onChange={(event) =>
                          setConfirmHistoryLoss(event.target.checked)
                        }
                      />
                      <span>{t("sync.keyHistoryLossConfirm")}</span>
                    </label>
                  ) : null}
                  <Button
                    kind="primary"
                    disabled={
                      busy ||
                      (chooseOwn && ownKey === "") ||
                      (status.keyConfigured && !confirmHistoryLoss)
                    }
                    onClick={() =>
                      void run(
                        () =>
                          status.keyConfigured
                            ? api.setSyncKey(
                                chooseOwn ? ownKey : undefined,
                                true,
                              )
                            : api.setSyncKey(chooseOwn ? ownKey : undefined),
                        (next) => {
                          setRevealed(chooseOwn ? "" : next.key);
                          setOwnKey("");
                          setConfirmHistoryLoss(false);
                          setNotice(t("sync.keySaved"));
                          void reload();
                        },
                        t("sync.keyFailed"),
                      )
                    }
                    className="self-start"
                  >
                    {status.keyConfigured
                      ? t("sync.keyReplace")
                      : t("sync.keyCreate")}
                  </Button>
                </div>
              </div>
            </section>
          ) : null}
        </div>
      </details>

      {transferPanel}

      {status.configured ? (
        <details className="group overflow-hidden rounded-md border border-control-line bg-card">
          <summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-3 bg-toolbar px-4 py-3 marker:hidden hover:bg-select-fill">
            <span className="flex items-center gap-3 text-sm font-medium text-ink">
              <span
                aria-hidden="true"
                className="inline-flex size-5 shrink-0 items-center justify-center text-base text-ink-muted transition-transform group-open:rotate-90"
              >
                ›
              </span>
              {t("sync.detailsHeading")}
            </span>
            <span className={hintText}>
              {status.synced
                ? t("sync.lastSynced", {
                    at: status.lastSyncedAt ?? "",
                    count: status.fileCount ?? 0,
                  })
                : t("sync.neverSynced")}
            </span>
          </summary>
          <div className="grid gap-4 border-t border-line bg-surface-subtle p-4 lg:grid-cols-2">
            <section
              aria-labelledby="sync-bucket-state-heading"
              className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4 lg:col-span-2"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h4 id="sync-bucket-state-heading" className={sectionHeading}>
                    {t("sync.bucketStateHeading")}
                  </h4>
                  <p className={`mt-1 ${hintText}`}>
                    {t("sync.bucketStateHint")}
                  </p>
                </div>
                <Button
                  disabled={busy || bucketState.phase === "loading"}
                  onClick={() => void refreshBucket()}
                >
                  {t("sync.bucketRefresh")}
                </Button>
              </div>

              {bucketState.phase === "idle" ? (
                <p className={hintText}>{t("sync.bucketNotConfigured")}</p>
              ) : bucketState.phase === "loading" ? (
                <p role="status" className={hintText}>
                  {t("sync.bucketLoading")}
                </p>
              ) : bucketState.phase === "error" ? (
                <Notice tone="danger">{bucketState.message}</Notice>
              ) : (
                <div className="grid gap-3 border-t border-line pt-3 lg:grid-cols-2">
                  <div className="rounded border border-line bg-surface p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
                      {t("sync.bucketLive")}
                    </p>
                    {bucketState.value.live === undefined ? (
                      <p className="mt-2 text-sm text-ink-muted">
                        {t("sync.bucketLiveEmpty")}
                      </p>
                    ) : (
                      <div className="mt-2 flex flex-col gap-1">
                        <p className={hintText}>
                          {t("sync.bucketObjectMeta", {
                            size: formatBytes(
                              bucketState.value.live.size,
                              locale,
                            ),
                            at: bucketState.value.live.lastModified ?? "—",
                          })}
                        </p>
                        <p
                          className={`text-sm font-medium ${bucketState.value.localIsLive ? "text-success" : "text-notice-ink"}`}
                        >
                          {t(
                            bucketState.value.localIsLive
                              ? "sync.bucketLocalCurrent"
                              : "sync.bucketLocalBehind",
                          )}
                        </p>
                        <details className="mt-1 text-xs text-ink-muted">
                          <summary className="cursor-pointer">
                            {t("sync.bucketObjectName")}
                          </summary>
                          <p className="mt-1 break-all font-mono text-ink">
                            {bucketState.value.live.key}
                          </p>
                        </details>
                      </div>
                    )}
                  </div>

                  <div className="rounded border border-line bg-surface p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
                      {t("sync.bucketHistory", {
                        count: bucketState.value.history.length,
                      })}
                    </p>
                    {bucketState.value.historyTruncated ? (
                      <p className="mt-2 text-xs text-notice-ink">
                        {t("sync.bucketHistoryTruncated")}
                      </p>
                    ) : null}
                    {bucketState.value.history.length === 0 ? (
                      <p className="mt-2 text-sm text-ink-muted">
                        {t("sync.bucketHistoryEmpty")}
                      </p>
                    ) : (
                      <div className="mt-2 flex flex-col gap-2">
                        <p className={hintText}>
                          {t("sync.bucketHistoryShowing", {
                            shown: visibleBucketHistory.length,
                            count: bucketState.value.history.length,
                          })}
                        </p>
                        <ul className="max-h-64 space-y-2 overflow-auto pr-1">
                          {visibleBucketHistory.map((item) => (
                            <li
                              key={item.key}
                              className="rounded border border-hairline px-2 py-1.5"
                            >
                              <p className={hintText}>
                                {t("sync.bucketObjectMeta", {
                                  size: formatBytes(item.size, locale),
                                  at: item.lastModified ?? "—",
                                })}
                              </p>
                              <details className="mt-1 text-xs text-ink-muted">
                                <summary className="cursor-pointer">
                                  {t("sync.bucketObjectName")}
                                </summary>
                                <p className="mt-1 break-all font-mono text-ink">
                                  {item.key}
                                </p>
                              </details>
                            </li>
                          ))}
                        </ul>
                        {bucketState.value.history.length <= 5 ? null : (
                          <Button
                            onClick={toggleBucketHistory}
                            className="self-start"
                          >
                            {t(
                              bucketHistoryExpanded
                                ? "sync.bucketHistoryCollapse"
                                : "sync.bucketHistoryExpand",
                            )}
                          </Button>
                        )}
                      </div>
                    )}
                  </div>

                  <p className={`lg:col-span-2 ${hintText}`}>
                    {t("sync.bucketCheckedAt", {
                      at: bucketState.value.checkedAt,
                    })}
                  </p>
                </div>
              )}
            </section>

            <SyncHistorySection
              busy={busy}
              direction={status.direction}
              historyDiff={historyDiff}
              historyState={historyState}
              keyConfigured={status.keyConfigured}
              locale={locale}
              selectedKey={selectedHistoryKey}
              t={t}
              onPreview={(key) => void previewWith(undefined, key)}
              onRefresh={() => void refreshHistory()}
              onSelect={(key) => void selectHistory(key)}
            />
          </div>
        </details>
      ) : null}

      {forcePushOpen ? (
        <SyncForcePushDialog
          busy={busy}
          keyConfigured={status.keyConfigured}
          message={pushMessage}
          t={t}
          onMessageChange={editPushMessage}
          onClose={() => setForcePushOpen(false)}
          onSubmit={() =>
            void run(
              () => api.forcePushSnapshot(pushMessage.trim()),
              (next) => {
                setStatusState({ phase: "ready", value: next.status });
                pull.close();
                setResultView({ kind: "push", result: next.result });
                setForcePushOpen(false);
                setNotice(t("sync.forcePushed"));
                acceptPushMessage();
                void refreshPushDraft();
                void refreshBucket();
                void refreshHistory();
              },
              t("sync.forceFailed"),
            )
          }
        />
      ) : null}

      {resultView !== null ? (
        <SyncResultCard view={resultView} />
      ) : status.lastOperation === undefined ? null : (
        <SyncResultCard
          view={{ kind: "previous", operation: status.lastOperation }}
        />
      )}

      {preview === null ? null : (
        <SyncPullPreviewDialog
          preview={preview}
          acceptRemoteHead={previewAcceptRemoteHead}
          acceptedRemovals={acceptedRemovals}
          busy={busy}
          direction={status.direction}
          t={t}
          onAcceptRemovals={pull.acceptRemovals}
          onClose={pull.close}
          onResolve={(choice) => void previewWith(choice, previewHistoryKey)}
          onApply={() =>
            void run(
              () =>
                previewAcceptRemoteHead
                  ? api.pullSnapshot(
                      true,
                      resolve,
                      previewHistoryKey,
                      preview,
                      true,
                    )
                  : api.pullSnapshot(true, resolve, previewHistoryKey, preview),
              (next) => {
                pull.replace(next);
                setResultView({ kind: "apply", result: next });
                setNotice(t("sync.applied"));
                void reload();
              },
              t("sync.applyFailed"),
            )
          }
        />
      )}
    </div>
  );
}

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type IntegrationsApi,
  type PullResponse,
  type SyncBucketStatus,
  type SyncDirection,
  type SyncHistory,
  type SyncHistoryDiff,
  type SyncPushDraft,
  type SyncSetupCheckResponse,
  type SyncStatus,
} from "../api/integrations";
import { useLanguage } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import {
  formatBytes,
  SyncResultCard,
  type SyncResultView,
} from "./SyncResultCard";

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

type SyncStatusState =
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncStatus };

type BucketStatusState =
  | { phase: "idle" }
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncBucketStatus };

type HistoryState =
  | { phase: "idle" }
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncHistory };

const refusals: Record<string, MessageKey> = {
  wrong_master_password: "sync.wrongMaster",
  wrong_passphrase: "sync.wrongKey",
  sync_key_missing: "sync.keyMissing",
  passphrase_too_short: "sync.keyTooShort",
  bucket_refused: "sync.unreachable",
  sync_failed: "sync.failed",
  endpoint_must_have_no_path: "sync.endpointPath",
  sync_remote_moved: "sync.remoteMoved",
  sync_remote_deleted: "sync.remoteDeleted",
  sync_key_recovery_required: "sync.keyRecoveryRequired",
  sync_key_recovery_target_change: "sync.keyRecoveryTargetChange",
  sync_history_key_loss_confirmation_required: "sync.keyHistoryLossConfirm",
  preview_stale: "sync.previewStale",
  sync_nothing_to_push: "sync.noLocalChanges",
  sync_commit_message_invalid: "sync.commitMessageInvalid",
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
  const [endpoint, setEndpoint] = useState("");
  const [bucket, setBucket] = useState("");
  const [path, setPath] = useState("");
  const [region, setRegion] = useState("");
  const [accessKeyId, setAccessKeyId] = useState("");
  const [secretAccessKey, setSecretAccessKey] = useState("");
  const [direction, setDirection] = useState<SyncDirection>("both");
  const [setupCheck, setSetupCheck] = useState<SyncSetupCheckResponse | null>(
    null,
  );
  const [master, setMaster] = useState("");
  const [revealed, setRevealed] = useState("");
  const [ownKey, setOwnKey] = useState("");
  const [chooseOwn, setChooseOwn] = useState(false);
  const [confirmHistoryLoss, setConfirmHistoryLoss] = useState(false);
  const [bucketState, setBucketState] = useState<BucketStatusState>({
    phase: "idle",
  });
  const [historyState, setHistoryState] = useState<HistoryState>({
    phase: "idle",
  });
  const [historyDiff, setHistoryDiff] = useState<SyncHistoryDiff | null>(null);
  const [selectedHistoryKey, setSelectedHistoryKey] = useState<string | null>(
    null,
  );
  const [previewHistoryKey, setPreviewHistoryKey] = useState<
    string | undefined
  >(undefined);
  const [previewAcceptRemoteHead, setPreviewAcceptRemoteHead] = useState(false);
  const [forceConfirmed, setForceConfirmed] = useState(false);
  const [acceptedRemovals, setAcceptedRemovals] = useState(false);
  const [resolve, setResolve] = useState<"local" | "remote" | undefined>(
    undefined,
  );
  const [preview, setPreview] = useState<PullResponse | null>(null);
  const [resultView, setResultView] = useState<SyncResultView | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [editingSettings, setEditingSettings] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [pushDraft, setPushDraft] = useState<SyncPushDraft | null>(null);
  const [pushMessage, setPushMessage] = useState("");
  const pushMessageDirty = useRef(false);

  function editSettings(current: SyncStatus) {
    setEndpoint(current.endpoint ?? "");
    setBucket(current.bucket ?? "");
    setPath(current.path ?? "");
    setRegion(current.region ?? "");
    setDirection(current.direction);
    setAccessKeyId("");
    setSecretAccessKey("");
    setSetupCheck(null);
    setEditingSettings(true);
    setSettingsOpen(true);
  }

  const setupInput = {
    endpoint,
    bucket,
    path,
    region,
    accessKeyId,
    secretAccessKey,
  };

  const refreshBucket = useCallback(async () => {
    setBucketState({ phase: "loading" });
    try {
      setBucketState({ phase: "ready", value: await api.syncBucketStatus() });
    } catch {
      setBucketState({ phase: "error", message: t("sync.bucketStatusFailed") });
    }
  }, [api, t]);

  const refreshHistory = useCallback(async () => {
    setHistoryState({ phase: "loading" });
    try {
      const value = await api.syncHistory();
      setHistoryState({ phase: "ready", value });
      setSelectedHistoryKey((current) =>
        current !== null &&
        value.revisions.some((revision) => revision.key === current)
          ? current
          : null,
      );
    } catch {
      setHistoryState({ phase: "error", message: t("sync.historyFailed") });
    }
  }, [api, t]);

  const refreshPushDraft = useCallback(async () => {
    try {
      const draft = await api.syncPushDraft();
      setPushDraft(draft);
      if (!pushMessageDirty.current) setPushMessage(draft.message);
    } catch {
      setPushDraft(null);
    }
  }, [api]);

  const reload = useCallback(async () => {
    setStatusState({ phase: "loading" });
    try {
      const next = await api.syncStatus();
      setStatusState({ phase: "ready", value: next });
      if (next.configured) void refreshBucket();
      else setBucketState({ phase: "idle" });
      if (next.configured && next.keyConfigured) void refreshHistory();
      else setHistoryState({ phase: "idle" });
      if (next.configured) void refreshPushDraft();
      else setPushDraft(null);
    } catch {
      setStatusState({ phase: "error", message: t("sync.statusFailed") });
    }
  }, [api, refreshBucket, refreshHistory, refreshPushDraft, t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    setSetupCheck(null);
  }, [endpoint, bucket, path, region, accessKeyId, secretAccessKey]);

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
    setError("");
    setNotice("");
    setBusy(true);
    try {
      apply(await operation());
    } catch (caught) {
      const code = failureCode(caught);
      const named = refusals[code];
      setError(explain?.(code) || (named === undefined ? failure : t(named)));
    } finally {
      setBusy(false);
    }
  }

  async function previewWith(choice?: "local" | "remote", historyKey?: string) {
    setResolve(choice);
    setPreviewHistoryKey(historyKey);
    setPreviewAcceptRemoteHead(false);
    await run(
      () => api.pullSnapshot(false, choice, historyKey),
      (next) => {
        setPreview(next);
        setAcceptedRemovals(false);
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
    setResolve("remote");
    setPreviewHistoryKey(undefined);
    setPreviewAcceptRemoteHead(true);
    await run(
      () => api.pullSnapshot(false, "remote", undefined, undefined, true),
      (next) => {
        setPreview(next);
        setAcceptedRemovals(false);
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
    setSelectedHistoryKey(key);
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
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
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

  const conflicted = (preview?.conflicts ?? []).length > 0;
  const remoteHeadBlocked =
    status.auto.phase === "blocked" && status.auto.detail === "remote_moved";
  const pushChangeCount =
    pushDraft === null
      ? null
      : pushDraft.added + pushDraft.modified + pushDraft.removed;
  const selectedHistory =
    historyState.phase === "ready" && selectedHistoryKey !== null
      ? historyState.value.revisions.find(
          (revision) => revision.key === selectedHistoryKey,
        )
      : undefined;

  return (
    <div
      className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
    >
      <PageHeader
        title={t("sync.heading")}
        description={t("sync.pageDescription")}
      />
      <dl className="sshc-card grid overflow-hidden rounded-md bg-toolbar sm:grid-cols-3">
        {[
          [
            t("sync.metricConfiguration"),
            t(
              status.configured
                ? "sync.stateConfigured"
                : "sync.stateNotConfigured",
            ),
          ],
          [t("sync.metricDirection"), t(`sync.direction.${status.direction}`)],
          [
            t("sync.metricSnapshot"),
            status.synced ? (status.fileCount ?? 0) : "—",
          ],
        ].map(([label, value]) => (
          <div
            key={String(label)}
            className="flex min-w-0 items-center justify-between gap-4 border-t border-hairline px-4 py-2.5 first:border-t-0 sm:border-l sm:border-t-0 sm:first:border-l-0"
          >
            <dt className="text-xs font-medium text-ink-muted">{label}</dt>
            <dd className="min-w-0 text-right font-mono text-sm font-semibold text-ink">
              {value}
            </dd>
          </div>
        ))}
      </dl>

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

      <p className={hintText}>{t("sync.warning")}</p>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
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
                  : status.auto.phase === "failed"
                    ? t(
                        status.auto.detail === "wrong_passphrase"
                          ? "sync.autoFailedWrongKey"
                          : status.auto.detail === "snapshot_schema_unsupported"
                            ? "sync.autoFailedSchema"
                            : "sync.autoFailedLast",
                      )
                    : status.auto.at === undefined
                      ? t("sync.autoIdle")
                      : t("sync.autoLastRan", { at: status.auto.at })}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              {remoteHeadBlocked && status.direction === "pull" ? (
                <Button
                  kind="primary"
                  disabled={busy || !status.keyConfigured}
                  onClick={() => void previewCurrentRemoteHead()}
                >
                  {t("sync.remoteHeadReview")}
                </Button>
              ) : (
                <Button
                  kind="primary"
                  disabled={busy || !status.keyConfigured}
                  onClick={() =>
                    void run(
                      () => api.syncNow(),
                      (next) => setStatusState({ phase: "ready", value: next }),
                      t("sync.autoNowFailed"),
                    )
                  }
                >
                  {t("sync.autoNow")}
                </Button>
              )}
              {status.direction === "push" || remoteHeadBlocked ? null : (
                <Button
                  disabled={busy || !status.keyConfigured}
                  onClick={() => void previewWith(undefined)}
                >
                  {t("sync.receiveRemote")}
                </Button>
              )}
            </div>
          </div>
        </section>
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

      {status.configured ? (
        <section className="sshc-card overflow-hidden rounded-md bg-card">
          <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
            <div className="flex items-center gap-2">
              <Icon name="sync" className="h-4 w-4 text-ink-muted" />
              <h3 className={sectionHeading}>{t("sync.snapshotHeading")}</h3>
            </div>
            <p className={hintText}>
              {status.synced
                ? t("sync.lastSynced", {
                    at: status.lastSyncedAt ?? "",
                    count: status.fileCount ?? 0,
                  })
                : t("sync.neverSynced")}
            </p>
          </header>
          <div className="grid gap-4 p-4 lg:grid-cols-2">
            <section
              aria-labelledby="sync-transfer-heading"
              className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4"
            >
              <h4 id="sync-transfer-heading" className={sectionHeading}>
                {t("sync.transferHeading")}
              </h4>
              <p className="text-sm leading-6 text-ink-muted">
                {t("sync.transferHint")}
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
                    pushMessageDirty.current = true;
                    setPushMessage(event.target.value);
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
                    !status.configured ||
                    !status.keyConfigured ||
                    status.direction === "pull" ||
                    pushMessage.trim() === "" ||
                    pushChangeCount === null ||
                    pushChangeCount === 0
                  }
                  onClick={() =>
                    void run(
                      () => api.pushSnapshot(pushMessage.trim()),
                      (next) => {
                        setStatusState({
                          phase: "ready",
                          value: next.status,
                        });
                        setPreview(null);
                        setResultView({
                          kind: "push",
                          result: next.result,
                        });
                        setNotice(t("sync.pushed"));
                        pushMessageDirty.current = false;
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
                  disabled={busy || !status.configured || !status.keyConfigured}
                  onClick={() => void previewWith(undefined)}
                >
                  {t("sync.preview")}
                </Button>
              </div>
            </section>

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
                        <p className="break-all font-mono text-xs text-ink">
                          {bucketState.value.live.key}
                        </p>
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
                      <ul className="mt-2 max-h-48 space-y-2 overflow-auto pr-1">
                        {bucketState.value.history.map((item) => (
                          <li
                            key={item.key}
                            className="rounded border border-hairline px-2 py-1.5"
                          >
                            <p className="break-all font-mono text-xs text-ink">
                              {item.key}
                            </p>
                            <p className={hintText}>
                              {t("sync.bucketObjectMeta", {
                                size: formatBytes(item.size, locale),
                                at: item.lastModified ?? "—",
                              })}
                            </p>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>

                  {bucketState.value.live === undefined ||
                  status.direction === "pull" ? null : (
                    <details className="rounded border border-danger/40 bg-danger/5 p-3 lg:col-span-2">
                      <summary className="cursor-pointer text-sm font-medium text-danger">
                        {t("sync.forceHeading")}
                      </summary>
                      <div className="mt-3 flex flex-col gap-3 border-t border-danger/30 pt-3">
                        <p className="text-sm leading-6 text-ink-muted">
                          {t("sync.forceHint")}
                        </p>
                        <label className="flex items-start gap-2 text-sm text-ink">
                          <input
                            type="checkbox"
                            checked={forceConfirmed}
                            onChange={(event) =>
                              setForceConfirmed(event.target.checked)
                            }
                            className="mt-0.5"
                          />
                          <span>{t("sync.forceConfirm")}</span>
                        </label>
                        <Button
                          kind="danger"
                          disabled={
                            busy ||
                            !forceConfirmed ||
                            !status.keyConfigured ||
                            pushMessage.trim() === ""
                          }
                          onClick={() =>
                            void run(
                              () => api.forcePushSnapshot(pushMessage.trim()),
                              (next) => {
                                setStatusState({
                                  phase: "ready",
                                  value: next.status,
                                });
                                setPreview(null);
                                setResultView({
                                  kind: "push",
                                  result: next.result,
                                });
                                setForceConfirmed(false);
                                setNotice(t("sync.forcePushed"));
                                pushMessageDirty.current = false;
                                void refreshPushDraft();
                                void refreshBucket();
                                void refreshHistory();
                              },
                              t("sync.forceFailed"),
                            )
                          }
                        >
                          {t("sync.forcePush")}
                        </Button>
                      </div>
                    </details>
                  )}

                  <p className={`lg:col-span-2 ${hintText}`}>
                    {t("sync.bucketCheckedAt", {
                      at: bucketState.value.checkedAt,
                    })}
                  </p>
                </div>
              )}
            </section>

            <section
              aria-labelledby="sync-history-heading"
              className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4 lg:col-span-2"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h4 id="sync-history-heading" className={sectionHeading}>
                    {t("sync.historyHeading")}
                  </h4>
                  <p className={`mt-1 ${hintText}`}>{t("sync.historyHint")}</p>
                </div>
                <Button
                  disabled={
                    busy ||
                    !status.keyConfigured ||
                    historyState.phase === "loading"
                  }
                  onClick={() => void refreshHistory()}
                >
                  {t("sync.historyRefresh")}
                </Button>
              </div>

              {historyState.phase === "idle" ? (
                <p className={hintText}>{t("sync.historyNeedsKey")}</p>
              ) : historyState.phase === "loading" ? (
                <p role="status" className={hintText}>
                  {t("sync.historyLoading")}
                </p>
              ) : historyState.phase === "error" ? (
                <Notice tone="danger">{historyState.message}</Notice>
              ) : (
                <div className="border-t border-line pt-3">
                  <p className={hintText}>
                    {t("sync.historySummary", {
                      count: historyState.value.revisions.length,
                      size: formatBytes(
                        historyState.value.downloadedBytes,
                        locale,
                      ),
                    })}
                  </p>
                  {historyState.value.historyTruncated ||
                  historyState.value.downloadTruncated ? (
                    <p className="mt-1 text-xs text-notice-ink">
                      {t("sync.historyTruncated")}
                    </p>
                  ) : null}
                  {historyState.value.skipped > 0 ? (
                    <p className="mt-1 text-xs text-notice-ink">
                      {t("sync.historySkipped", {
                        count: historyState.value.skipped,
                      })}
                    </p>
                  ) : null}

                  <div className="mt-3 grid gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
                    <ol
                      className="max-h-80 space-y-2 overflow-auto pr-1"
                      aria-label={t("sync.historyTimeline")}
                    >
                      {historyState.value.revisions.map((revision) => (
                        <li
                          key={revision.key}
                          className="relative border-l-2 border-line pl-4"
                        >
                          <span className="absolute -left-[5px] top-4 h-2 w-2 rounded-full bg-accent" />
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => void selectHistory(revision.key)}
                            className={`w-full rounded border px-3 py-2 text-left ${
                              selectedHistoryKey === revision.key
                                ? "border-accent bg-select-fill"
                                : "border-hairline bg-surface hover:border-line"
                            }`}
                          >
                            <span className="flex flex-wrap items-center justify-between gap-2">
                              <span className="font-mono text-xs font-semibold text-ink">
                                {revision.revision.slice(0, 12)}
                              </span>
                              <span className="rounded-full bg-toolbar px-2 py-0.5 text-[11px] font-medium text-ink-muted">
                                {t(
                                  `sync.historyRelation.${revision.relation}` as MessageKey,
                                )}
                              </span>
                            </span>
                            <span className="mt-1 block text-xs text-ink-muted">
                              {t("sync.historyRevisionMeta", {
                                at: revision.createdAt,
                                count: revision.fileCount,
                                origin: revision.origin.slice(0, 8),
                              })}
                            </span>
                            <span className="mt-1 block text-sm font-medium text-ink">
                              {revision.message}
                            </span>
                            {revision.parentRevision === undefined ? null : (
                              <span className="mt-1 block font-mono text-[11px] text-ink-muted">
                                {t("sync.historyParent", {
                                  revision: revision.parentRevision.slice(
                                    0,
                                    12,
                                  ),
                                })}
                              </span>
                            )}
                          </button>
                        </li>
                      ))}
                    </ol>

                    <div className="min-w-0 rounded border border-line bg-surface p-3">
                      {selectedHistory === undefined ? (
                        <p className={hintText}>{t("sync.historySelect")}</p>
                      ) : (
                        <div className="flex flex-col gap-3">
                          <div>
                            <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
                              {t("sync.historySelected")}
                            </p>
                            <p className="mt-1 break-all font-mono text-xs text-ink">
                              {selectedHistory.key}
                            </p>
                          </div>
                          {historyDiff === null ? (
                            <p role="status" className={hintText}>
                              {busy
                                ? t("sync.historyDiffLoading")
                                : t("sync.historyDiffEmpty")}
                            </p>
                          ) : (
                            <div className="grid gap-2 text-xs sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
                              {(
                                [
                                  ["added", historyDiff.added, "text-success"],
                                  [
                                    "modified",
                                    historyDiff.modified,
                                    "text-notice-ink",
                                  ],
                                  [
                                    "removed",
                                    historyDiff.removed,
                                    "text-danger",
                                  ],
                                ] as const
                              ).map(([kind, paths, tone]) => (
                                <div
                                  key={kind}
                                  className="min-w-0 rounded bg-toolbar p-2"
                                >
                                  <p className={`font-medium ${tone}`}>
                                    {t(
                                      `sync.historyDiff.${kind}` as MessageKey,
                                      { count: paths.length },
                                    )}
                                  </p>
                                  <ul className="mt-1 max-h-24 space-y-0.5 overflow-auto font-mono text-[11px] text-ink-muted">
                                    {paths.map((path) => (
                                      <li key={path} className="break-all">
                                        {path}
                                      </li>
                                    ))}
                                  </ul>
                                </div>
                              ))}
                            </div>
                          )}
                          <p className={hintText}>
                            {t("sync.historyRestoreHint")}
                          </p>
                          <Button
                            kind="primary"
                            disabled={busy || status.direction === "push"}
                            onClick={() =>
                              void previewWith(undefined, selectedHistory.key)
                            }
                            className="self-start"
                          >
                            {t("sync.historyRestorePreview")}
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}
            </section>
          </div>
          {status.direction === "both" ? null : (
            <p
              role="status"
              className="border-t border-notice-line bg-notice px-4 py-3 text-sm text-notice-ink"
            >
              {t(`sync.direction.${status.direction}.active`)}
            </p>
          )}
        </section>
      ) : null}

      {resultView !== null ? (
        <SyncResultCard view={resultView} />
      ) : status.lastOperation === undefined ? null : (
        <SyncResultCard
          view={{ kind: "previous", operation: status.lastOperation }}
        />
      )}

      {preview === null ? null : (
        <section className="sshc-card overflow-hidden rounded-md bg-card">
          <header className="flex items-center gap-2 border-b border-line bg-toolbar px-4 py-3">
            <Icon name="config" className="h-4 w-4 text-ink-muted" />
            <h3 className={sectionHeading}>
              {t(
                previewAcceptRemoteHead
                  ? "sync.remoteHeadPreviewHeading"
                  : "sync.previewHeading",
              )}
            </h3>
          </header>
          <div className="flex flex-col gap-3 px-4 py-4">
            {previewAcceptRemoteHead ? (
              <Notice tone="notice">
                {t("sync.remoteHeadPreview", {
                  at: preview.summary.createdAt,
                  origin: preview.origin ?? "—",
                })}
              </Notice>
            ) : null}
            {conflicted ? (
              <>
                <p className="text-sm text-notice-ink">
                  {t("sync.conflictExplain")}
                </p>
                <ul className="flex flex-col gap-1 font-mono text-xs text-notice-ink">
                  {preview.conflicts.map((conflict) => (
                    <li key={conflict.path}>{conflict.path}</li>
                  ))}
                </ul>

                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void previewWith("local", previewHistoryKey)}
                    className="rounded border border-line px-3 py-1.5 text-sm text-ink"
                  >
                    {t("sync.keepMine")}
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      void previewWith("remote", previewHistoryKey)
                    }
                    className="rounded border border-line px-3 py-1.5 text-sm text-ink"
                  >
                    {t("sync.takeTheirs")}
                  </button>
                </div>
              </>
            ) : null}
            {preview.written.length === 0 ? null : (
              <>
                <p className={hintText}>
                  {t("sync.wouldWrite", { count: preview.written.length })}
                </p>
                <ul className="flex flex-col gap-1 font-mono text-xs text-ink-muted">
                  {preview.written.map((path) => (
                    <li key={path}>{path}</li>
                  ))}
                </ul>
              </>
            )}
            {preview.removed.length === 0 ? null : (
              <>
                <p className={hintText}>
                  {t("sync.wouldRemove", { count: preview.removed.length })}
                </p>
                <ul className="flex flex-col gap-1 font-mono text-xs text-danger">
                  {preview.removed.map((path) => (
                    <li key={path}>{path}</li>
                  ))}
                </ul>
              </>
            )}
            {preview.removed.length === 0 ? null : (
              <label className="flex items-start gap-2 rounded border border-notice-line bg-notice p-3 text-sm text-notice-ink">
                <input
                  type="checkbox"
                  checked={acceptedRemovals}
                  onChange={(event) =>
                    setAcceptedRemovals(event.target.checked)
                  }
                  className="mt-0.5"
                />
                <span>{t("sync.confirmOverwrite")}</span>
              </label>
            )}
            <Button
              kind="primary"
              disabled={
                busy ||
                conflicted ||
                status.direction === "push" ||
                (preview.removed.length > 0 && !acceptedRemovals)
              }
              onClick={() =>
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
                      : api.pullSnapshot(
                          true,
                          resolve,
                          previewHistoryKey,
                          preview,
                        ),
                  (next) => {
                    setPreview(next);
                    setResultView({ kind: "apply", result: next });
                    setNotice(t("sync.applied"));
                    void reload();
                  },
                  t("sync.applyFailed"),
                )
              }
              className="self-start"
            >
              {t(
                previewAcceptRemoteHead ? "sync.remoteHeadApply" : "sync.apply",
              )}
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}

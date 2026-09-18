import { useCallback, useEffect, useState } from "react";
import { vaultApi, type VaultApi } from "../api/vault";
import { syncApi, type SyncApi, type PushResponse, type SyncHistoryDiff, type SyncStatus } from "../api/sync";
import { useLanguage, useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PanelState } from "../ui/PanelState";
import { PageHeader } from "../ui/page";
import { usePolling } from "../ui/usePolling";
import { SyncResultCard } from "./SyncResultCard";
import { SyncExclusionsPanel } from "./SyncExclusionsPanel";
import { useSyncSetupForm } from "./useSyncSetupForm";
import { useSyncRemoteState } from "./useSyncRemoteState";
import { useSyncOperation } from "./useSyncOperation";
import { useSyncPullPreview } from "./useSyncPullPreview";
import { SyncForcePushDialog } from "./SyncForcePushDialog";
import { SyncPullPreviewDialog } from "./SyncPullPreviewDialog";
import { SyncHistorySection } from "./SyncHistorySection";
import { SyncBucketStateSection } from "./SyncBucketStateSection";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { SyncOverviewCard } from "./SyncOverviewCard";
import { SyncSettingsSection } from "./SyncSettingsSection";
import { SyncTransferCard } from "./SyncTransferCard";
import { SyncUnlockCard } from "./SyncUnlockCard";
import { syncRefusals } from "./syncRefusals";

// Another device's push shows up within half a minute; the bucket listing
// is a paid request, so the panel does not ask more often.
const bucketPollIntervalMs = 30_000;

// Setting the shared key needs the vault open; everything else is sync.
export type SyncPanelApi = SyncApi & Pick<VaultApi, "unlockVault">;
export const syncPanelApi: SyncPanelApi = { ...syncApi, ...vaultApi };

type SyncPanelProps = { api?: SyncPanelApi };

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

// The three things to do, in order, before sync is set up.
function SyncFlowSteps() {
  const t = useTranslate();
  return (
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
  );
}

type SyncStatusState =
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncStatus };

export function SyncPanel({ api = syncPanelApi }: SyncPanelProps) {
  const { locale, t } = useLanguage();
  const [statusState, setStatusState] = useState<SyncStatusState>({
    phase: "loading",
  });
  const form = useSyncSetupForm();
  const [master, setMaster] = useState("");
  // A freshly generated key, shown once and never stored in the browser.
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
  const operation = useSyncOperation(t, syncRefusals);
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
  usePolling(refreshBucket, { intervalMs: bucketPollIntervalMs, enabled: shouldPollBucket });

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
    return <PanelState tone="loading" title={t("sync.loading")} />;
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
        <SyncUnlockCard
          master={master}
          busy={busy}
          onMasterChange={setMaster}
          onUnlock={() =>
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
        />
      </div>
    );
  }

  const remoteHeadBlocked =
    status.auto.phase === "blocked" && status.auto.detail === "remote_moved";

  // Pushed or force-pushed: the engine's status and result replace what
  // the screen was showing, and everything derived from the bucket reloads.
  function adoptPush(next: PushResponse, noticeKey: "sync.pushed" | "sync.forcePushed") {
    setStatusState({ phase: "ready", value: next.status });
    pull.close();
    setResultView({ kind: "push", result: next.result });
    setNotice(t(noticeKey));
    acceptPushMessage();
    void refreshPushDraft();
    void refreshBucket();
    void refreshHistory();
  }

  return (
    <div
      className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
    >
      <PageHeader
        title={t("sync.heading")}
        description={t("sync.pageDescription")}
      />

      {status.configured ? null : <SyncFlowSteps />}

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
        <SyncOverviewCard
          status={status}
          busy={busy}
          remoteHeadBlocked={remoteHeadBlocked}
          onToggleAuto={(checked) =>
            void run(
              () => api.setAutoSync(checked),
              (next) => setStatusState({ phase: "ready", value: next }),
              t("sync.autoFailed"),
            )
          }
          onSyncNow={() =>
            void run(
              () => api.syncNow(),
              (next) => setStatusState({ phase: "ready", value: next }),
              t("sync.autoNowFailed"),
            )
          }
          onPreviewRemoteHead={() => void previewCurrentRemoteHead()}
          onPreview={() => void previewWith(undefined)}
          onForcePush={() => setForcePushOpen(true)}
        />
      ) : null}

      {status.configured ? (
        <SyncExclusionsPanel
          api={api}
          onSaved={() => {
            void refreshPushDraft();
          }}
        />
      ) : null}

      <SyncSettingsSection
        status={status}
        busy={busy}
        form={form}
        onCheckSetup={() =>
          void run(
            () => api.checkSyncSetup(form.setupInput),
            (next) => {
              form.setSetupCheck(next);
              form.setOwnKey("");
              form.setChooseOwn(false);
            },
            t("sync.configureFailed"),
          )
        }
        onCompleteSetup={() => {
          const { setupCheck, chooseOwn, ownKey } = form;
          if (setupCheck === null) return;
          void run(
            () =>
              api.completeSyncSetup({
                ...form.setupInput,
                direction: form.direction,
                expectedState: setupCheck.state,
                ...(setupCheck.etag === undefined
                  ? {}
                  : { expectedETag: setupCheck.etag }),
                historyPresent: setupCheck.historyPresent,
                reuseKey: false,
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
              form.setOwnKey("");
              form.setAccessKeyId("");
              form.setSecretAccessKey("");
              form.setSetupCheck(null);
              form.setEditingSettings(false);
              form.setSettingsOpen(false);
              setNotice(
                next.generatedKey === undefined
                  ? t("sync.setup.saved")
                  : t("sync.keyShownOnce"),
              );
            },
            t("sync.configureFailed"),
          );
        }}
        onSaveKey={() => {
          const { chooseOwn, ownKey } = form;
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
              form.setOwnKey("");
              form.setConfirmHistoryLoss(false);
              setNotice(t("sync.keySaved"));
              void reload();
            },
            t("sync.keyFailed"),
          );
        }}
      />

      {status.configured && status.direction !== "pull" ? (
        <SyncTransferCard
          status={status}
          busy={busy}
          pushDraft={pushDraft}
          pushMessage={pushMessage}
          onMessageChange={editPushMessage}
          onPush={() =>
            void run(
              () => api.pushSnapshot(pushMessage.trim()),
              (next) => adoptPush(next, "sync.pushed"),
              t("sync.pushFailed"),
            )
          }
          onPreview={() => void previewWith(undefined)}
        />
      ) : null}

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
            <SyncBucketStateSection
              bucketState={bucketState}
              locale={locale}
              busy={busy}
              historyExpanded={bucketHistoryExpanded}
              onToggleHistory={toggleBucketHistory}
              onRefresh={() => void refreshBucket()}
            />

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
                adoptPush(next, "sync.forcePushed");
                setForcePushOpen(false);
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

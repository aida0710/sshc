import { useState } from "react";
import { vaultApi, type VaultApi } from "../api/vault";
import { syncApi, type SyncApi, type PushResponse, type SyncHistoryDiff } from "../api/sync";
import { useLanguage, useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Button, Notice } from "../ui/surface";
import { PanelState } from "../ui/PanelState";
import { SyncResultCard } from "./SyncResultCard";
import { SyncPageFrame } from "./SyncPageFrame";
import { SyncDetailsSection } from "./SyncDetailsSection";
import { useSyncStatus } from "./useSyncStatus";
import { SyncExclusionsPanel } from "./SyncExclusionsPanel";
import { useSyncSetupForm } from "./useSyncSetupForm";
import { useSyncRemoteState } from "./useSyncRemoteState";
import { useSyncOperation } from "./useSyncOperation";
import { useSyncPullPreview } from "./useSyncPullPreview";
import { SyncForcePushDialog } from "./SyncForcePushDialog";
import { SyncPullPreviewDialog } from "./SyncPullPreviewDialog";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { SyncOverviewCard } from "./SyncOverviewCard";
import { SyncSettingsSection } from "./SyncSettingsSection";
import { SyncTransferCard } from "./SyncTransferCard";
import { SyncUnlockCard } from "./SyncUnlockCard";
import { syncPathRefusals, syncRefusals } from "./syncRefusals";

// Setting the shared key needs the vault open; everything else is sync.
export type SyncPanelApi = SyncApi & Pick<VaultApi, "unlockVault">;
export const syncPanelApi: SyncPanelApi = { ...syncApi, ...vaultApi };

type SyncPanelProps = { api?: SyncPanelApi };

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

export function SyncPanel({ api = syncPanelApi }: SyncPanelProps) {
  const { locale, t } = useLanguage();
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
  const operation = useSyncOperation(t, syncRefusals, syncPathRefusals);
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
  const { statusState, reload, adopt: adoptStatus } = useSyncStatus({
    api,
    refreshBucket,
    resetBucket,
    refreshHistory,
    resetHistory,
    refreshPushDraft,
    resetPush,
    clearError,
  });

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
      <SyncPageFrame>
        <Notice tone="danger">{statusState.message}</Notice>
        <Button onClick={() => void reload()} className="self-start">
          {t("shell.bootstrapRetry")}
        </Button>
      </SyncPageFrame>
    );
  }

  const status = statusState.value;

  if (status.locked) {
    return (
      <SyncPageFrame>
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
      </SyncPageFrame>
    );
  }

  const remoteHeadBlocked =
    status.auto.phase === "blocked" && status.auto.detail === "remote_moved";

  // Pushed or force-pushed: the engine's status and result replace what
  // the screen was showing, and everything derived from the bucket reloads.
  function adoptPush(next: PushResponse, noticeKey: "sync.pushed" | "sync.forcePushed") {
    adoptStatus(next.status);
    pull.close();
    setResultView({ kind: "push", result: next.result });
    setNotice(t(noticeKey));
    acceptPushMessage();
    void refreshPushDraft();
    void refreshBucket();
    void refreshHistory();
  }

  return (
    <SyncPageFrame>
      {status.configured ? null : <SyncFlowSteps />}

      {/* 強制送信と適用のダイアログは、同じ失敗を自分の中に出す。背面にも出すと、
          スクリーンリーダーが同じ文を 2 回読み上げる。 */}
      {forcePushOpen || preview !== null ? null : <SyncErrorNotice message={error} code={errorCode} />}
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
              adoptStatus,
              t("sync.autoFailed"),
            )
          }
          onSyncNow={() =>
            void run(
              () => api.syncNow(),
              adoptStatus,
              t("sync.autoNowFailed"),
            )
          }
          onPreviewRemoteHead={() => void previewCurrentRemoteHead()}
          onPreview={() => void previewWith(undefined)}
          onForcePush={() => {
            // 前の操作の失敗をダイアログの中に持ち込まない。
            clearError();
            setForcePushOpen(true);
          }}
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
            form.acceptSetupCheck,
            t("sync.configureFailed"),
          )
        }
        onCompleteSetup={() => {
          const request = form.setupRequest();
          if (request === null) return;
          void run(
            () => api.completeSyncSetup(request),
            (next) => {
              adoptStatus(next.status);
              setRevealed(next.generatedKey ?? "");
              form.finishSetup();
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
        <SyncDetailsSection
          status={status}
          locale={locale}
          busy={busy}
          bucketState={bucketState}
          bucketHistoryExpanded={bucketHistoryExpanded}
          onToggleBucketHistory={toggleBucketHistory}
          onRefreshBucket={() => void refreshBucket()}
          historyState={historyState}
          historyDiff={historyDiff}
          selectedHistoryKey={selectedHistoryKey}
          onRefreshHistory={() => void refreshHistory()}
          onSelectHistory={(key) => void selectHistory(key)}
          onPreviewHistory={(key) => void previewWith(undefined, key)}
        />
      ) : null}

      {forcePushOpen ? (
        <SyncForcePushDialog
          busy={busy}
          error={error}
          errorCode={errorCode}
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
          error={error}
          errorCode={errorCode}
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
    </SyncPageFrame>
  );
}

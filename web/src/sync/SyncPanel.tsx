import { useState } from "react";
import { vaultApi, type VaultApi } from "../api/vault";
import { syncApi, type SyncApi, type PushResponse, type SyncHistoryDiff, type SyncSetupExport } from "../api/sync";
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
import { useSyncSetupImport } from "./useSyncSetupImport";
import { useSyncRemoteState } from "./useSyncRemoteState";
import { useSyncOperation, type SyncRunOptions } from "./useSyncOperation";
import { useSyncPullPreview } from "./useSyncPullPreview";
import { SyncForcePushDialog } from "./SyncForcePushDialog";
import { SyncPullPreviewDialog } from "./SyncPullPreviewDialog";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { SyncOverviewCard } from "./SyncOverviewCard";
import { SyncSettingsSection } from "./SyncSettingsSection";
import { SyncSetupExportSection } from "./SyncSetupExportSection";
import { SyncSetupImportSection } from "./SyncSetupImportSection";
import { SyncTransferCard } from "./SyncTransferCard";
import { SyncUnlockCard } from "./SyncUnlockCard";
import { syncPathRefusals, syncRefusals } from "./syncRefusals";

// Setting the shared key needs the vault open; everything else is sync.
export type SyncPanelApi = SyncApi & Pick<VaultApi, "unlockVault">;
const syncPanelApi: SyncPanelApi = { ...syncApi, ...vaultApi };

type SyncPanelProps = { api?: SyncPanelApi };

// The three things to do, in order, before sync is set up.
function SyncFlowSteps() {
  const t = useTranslate();
  return (
    <ol aria-label={t("sync.flowHeading")} className="grid gap-2 sm:grid-cols-3">
      {["sync.flowBucket", "sync.flowKey", "sync.flowOperate"].map(
        (key, index) => (
          // The steps show only before sync is set up, so the first one (the bucket) is the one at hand.
          <li
            key={key}
            aria-current={index === 0 ? "step" : undefined}
            className={`flex min-h-11 items-center gap-2.5 rounded-inset px-3 text-ui ${index === 0 ? "bg-select-fill text-ink" : "bg-well text-ink-muted"}`}
          >
            <span className={`flex size-[22px] shrink-0 items-center justify-center rounded-full font-mono text-xs font-semibold ${index === 0 ? "bg-accent text-accent-ink" : "bg-ink/10 text-ink"}`}>
              {index + 1}
            </span>
            {t(key as MessageKey)}
          </li>
        ),
      )}
    </ol>
  );
}

export function SyncPanel({ api = syncPanelApi }: SyncPanelProps) {
  const { locale, t } = useLanguage();
  const form = useSyncSetupForm();
  const importForm = useSyncSetupImport();
  // The settings sealed for another machine, while their QR code is on the screen.
  const [exported, setExported] = useState<SyncSetupExport | null>(null);
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
  const { statusState, reload, adopt: adoptStatus, adoptAndRefresh } = useSyncStatus({
    api,
    refreshBucket,
    resetBucket,
    refreshHistory,
    resetHistory,
    refreshPushDraft,
    resetPush,
    clearError,
  });

  async function run<T>(operation: () => Promise<T>, options: SyncRunOptions<T>) {
    await execute(operation, {
      ...options,
      recover: async (code) => {
        if (
          code !== "sync_remote_moved" ||
          statusState.phase !== "ready" ||
          statusState.value.direction !== "pull"
        ) {
          return false;
        }
        await reload();
        return true;
      },
    });
  }

  async function previewWith(choice?: "local" | "remote", historyKey?: string) {
    pull.prepare(choice, historyKey);
    await run(() => api.pullSnapshot(false, choice, historyKey), {
      apply: (next) => {
        pull.show(next);
        setResultView({ kind: "preview", result: next });
        setNotice(
          next.written.length + next.removed.length + next.conflicts.length ===
            0
            ? t("sync.alreadyMatches")
            : "",
        );
      },
      failure: t("sync.pullFailed"),
    });
  }

  async function previewCurrentRemoteHead() {
    pull.prepareRemoteHead();
    await run(() => api.pullSnapshot(false, "remote", undefined, undefined, true), {
      apply: (next) => {
        pull.show(next);
        setResultView({ kind: "preview", result: next });
        setNotice(
          next.written.length + next.removed.length === 0
            ? t("sync.alreadyMatches")
            : "",
        );
      },
      failure: t("sync.pullFailed"),
    });
  }

  async function selectHistory(key: string) {
    selectHistoryKey(key);
    setHistoryDiff(null);
    await run(() => api.diffSyncHistory(key), {
      apply: (next) => setHistoryDiff(next),
      failure: t("sync.historyDiffFailed"),
    });
  }

  if (statusState.phase === "loading") {
    return (
      <SyncPageFrame>
        <PanelState tone="loading" title={t("sync.loading")} />
      </SyncPageFrame>
    );
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
            void run(() => api.unlockVault(master), {
              apply: () => {
                setMaster("");
                void reload();
              },
              failure: t("sync.unlockFailed"),
              explain: (code) => (code === "vault_missing" ? t("sync.noVault") : ""),
            })
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
    pull.close();
    setResultView({ kind: "push", result: next.result });
    setNotice(t(noticeKey));
    // The message went out with this push, so the next draft may replace it.
    acceptPushMessage();
    adoptAndRefresh(next.status);
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
            void run(() => api.setAutoSync(checked), {
              apply: adoptStatus,
              failure: t("sync.autoFailed"),
            })
          }
          onSyncNow={() =>
            // The cycle sends its own message, so a message typed by hand stays for the next push.
            run(() => api.syncNow(), {
              apply: adoptAndRefresh,
              failure: t("sync.autoNowFailed"),
            })
          }
          onPreviewRemoteHead={previewCurrentRemoteHead}
          onPreview={() => previewWith(undefined)}
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

      {status.configured ? null : (
        <SyncSetupImportSection
          busy={busy}
          form={importForm}
          onImport={() =>
            void run(() => api.importSyncSetup(importForm.request), {
              apply: (next) => {
                adoptAndRefresh(next.status);
                importForm.reset();
                setNotice(t("sync.import.saved"));
              },
              failure: t("sync.import.failed"),
              // The shared wording for a wrong key speaks of the key saved on this machine; here it is the one just typed.
              explain: (code) => (code === "wrong_passphrase" ? t("sync.import.wrongKey") : ""),
            })
          }
        />
      )}

      {/* 書き出した設定を読み込んだあとは、手で入力する欄を出さない。同じ「同期の方向」が2つ並び、
          どちらを埋めるのか分からなくなる。［別の設定を選ぶ］で戻せる。 */}
      {!status.configured && importForm.document !== "" ? null : (
      <SyncSettingsSection
        status={status}
        busy={busy}
        form={form}
        exportSection={(
          <SyncSetupExportSection
            busy={busy}
            exported={exported}
            onExport={() =>
              void run(() => api.exportSyncSetup(), {
                apply: setExported,
                failure: t("sync.export.failed"),
              })
            }
            onClose={() => setExported(null)}
          />
        )}
        onCheckSetup={() =>
          void run(() => api.checkSyncSetup(form.setupInput), {
            apply: form.acceptSetupCheck,
            failure: t("sync.configureFailed"),
          })
        }
        onCompleteSetup={() => {
          const request = form.setupRequest();
          if (request === null) return;
          void run(() => api.completeSyncSetup(request), {
            apply: (next) => {
              adoptAndRefresh(next.status);
              setRevealed(next.generatedKey ?? "");
              form.finishSetup();
              // An export on the screen was sealed from the settings just replaced.
              setExported(null);
              setNotice(t("sync.setup.saved"));
            },
            failure: t("sync.configureFailed"),
          });
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
            {
              apply: (next) => {
                setRevealed(chooseOwn ? "" : next.key);
                // An export on the screen was sealed with the key just replaced.
                setExported(null);
                form.setOwnKey("");
                form.setConfirmHistoryLoss(false);
                setNotice(t("sync.keySaved"));
                void reload();
              },
              failure: t("sync.keyFailed"),
            },
          );
        }}
      />
      )}

      {status.configured && status.direction !== "pull" ? (
        <SyncTransferCard
          status={status}
          busy={busy}
          pushDraft={pushDraft}
          pushMessage={pushMessage}
          onMessageChange={editPushMessage}
          onPush={() =>
            void run(() => api.pushSnapshot(pushMessage.trim()), {
              apply: (next) => adoptPush(next, "sync.pushed"),
              failure: t("sync.pushFailed"),
            })
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
            void run(() => api.forcePushSnapshot(pushMessage.trim()), {
              apply: (next) => {
                adoptPush(next, "sync.forcePushed");
                setForcePushOpen(false);
              },
              failure: t("sync.forceFailed"),
            })
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
              {
                apply: (next) => {
                pull.replace(next);
                setResultView({ kind: "apply", result: next });
                setNotice(t("sync.applied"));
                void reload();
              },
                failure: t("sync.applyFailed"),
              },
            )
          }
        />
      )}
    </SyncPageFrame>
  );
}

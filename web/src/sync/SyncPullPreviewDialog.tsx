import type { PullResponse, SyncDirection } from "../api/sync";
import type { Translate } from "../i18n/context";
import { CheckboxField, hintText, sectionHeading } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { usePendingAction } from "../ui/usePendingAction";
import { ModalShell } from "../ui/ModalShell";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { ActionArea } from "../ui/ActionArea";

// 適用の失敗は、このダイアログの中に出す。背面の画面に出すとダイアログに隠れる。
type SyncPullPreviewDialogProps = {
  preview: PullResponse;
  acceptRemoteHead: boolean;
  acceptedRemovals: boolean;
  busy: boolean;
  error: string;
  errorCode: string;
  direction: SyncDirection;
  t: Translate;
  onAcceptRemovals: (accepted: boolean) => void;
  onApply: () => Promise<void>;
  onClose: () => void;
  onResolve: (choice: "local" | "remote") => Promise<void>;
};

export function SyncPullPreviewDialog({
  preview,
  acceptRemoteHead,
  acceptedRemovals,
  busy,
  error,
  errorCode,
  direction,
  t,
  onAcceptRemovals,
  onApply,
  onClose,
  onResolve,
}: SyncPullPreviewDialogProps) {
  const conflicted = preview.conflicts.length > 0;
  // 押したボタンだけを busy にする（SyncOverviewCard と同じ形）。失敗は SyncPanel が文にして出す。
  const pending = usePendingAction<"keepMine" | "takeTheirs" | "apply">();
  return (
    <ModalShell
      labelledBy="sync-pull-preview-heading"
      onDismiss={() => {
        if (!busy) onClose();
      }}
      placement="sheet"
      panelClassName="max-h-[90vh] w-full max-w-2xl overflow-auto"
    >
        <header className="flex items-center gap-2 border-b border-line bg-toolbar px-4 py-3">
          <Icon name="config" className="h-4 w-4 text-ink-muted" />
          <h3
            id="sync-pull-preview-heading"
            className={`${sectionHeading} flex-1`}
          >
            {t(
              acceptRemoteHead
                ? "sync.remoteHeadPreviewHeading"
                : "sync.previewHeading",
            )}
          </h3>
          <Button disabled={busy} onClick={onClose}>
            {t("sync.dialogClose")}
          </Button>
        </header>
        <div className="flex flex-col gap-3 px-4 py-4">
          <SyncErrorNotice message={error} code={errorCode} />
          {acceptRemoteHead ? (
            <Notice tone="notice">
              {t("sync.remoteHeadPreview", {
                at: preview.summary.createdAt,
                origin: preview.origin ?? "—",
              })}
            </Notice>
          ) : null}
          {conflicted ? (
            <Notice
              live={false}
              actions={(
                <>
                  <Button busy={pending.running === "keepMine"} busyLabel={t("busy.checking")} disabled={busy} onClick={() => void pending.start("keepMine", () => onResolve("local"))}>
                    {t("sync.keepMine")}
                  </Button>
                  <Button busy={pending.running === "takeTheirs"} busyLabel={t("busy.checking")} disabled={busy} onClick={() => void pending.start("takeTheirs", () => onResolve("remote"))}>
                    {t("sync.takeTheirs")}
                  </Button>
                </>
              )}
            >
              {t("sync.conflictExplain")}
              <ul className="mt-2 flex flex-col gap-2 text-xs">
                {preview.conflicts.map((conflict) => {
                  const modeChanged =
                    conflict.baseMode !== conflict.localMode ||
                    conflict.baseMode !== conflict.remoteMode;
                  return (
                    <li key={conflict.path} className="flex flex-col gap-0.5">
                      <span className="font-mono">{conflict.path}</span>
                      {modeChanged ? (
                        <span className="text-ink-muted">
                          {t("sync.conflictPermissions", {
                            base: conflict.baseMode ?? "—",
                            local: conflict.localMode ?? "—",
                            remote: conflict.remoteMode ?? "—",
                          })}
                        </span>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            </Notice>
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
              <CheckboxField
                label={t("sync.confirmOverwrite")}
                checked={acceptedRemovals}
                onChange={onAcceptRemovals}
                className="rounded-inset border border-notice-line bg-notice p-3"
              />
            </>
          )}
          <ActionArea placement="inline">
            <Button
              kind="primary"
              busy={pending.running === "apply"}
              busyLabel={t(acceptRemoteHead ? "busy.receiving" : "busy.applying")}
              disabled={
                busy ||
                conflicted ||
                direction === "push" ||
                (preview.removed.length > 0 && !acceptedRemovals)
              }
              onClick={() => void pending.start("apply", onApply)}
            >
              {t(acceptRemoteHead ? "sync.remoteHeadApply" : "sync.apply")}
            </Button>
          </ActionArea>
        </div>
    </ModalShell>
  );
}

import type { PullResponse, SyncDirection } from "../api/integrations";
import type { Translate } from "../i18n/context";
import { hintText, sectionHeading } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { ModalShell } from "../ui/ModalShell";

type SyncPullPreviewDialogProps = {
  preview: PullResponse;
  acceptRemoteHead: boolean;
  acceptedRemovals: boolean;
  busy: boolean;
  direction: SyncDirection;
  t: Translate;
  onAcceptRemovals: (accepted: boolean) => void;
  onApply: () => void;
  onClose: () => void;
  onResolve: (choice: "local" | "remote") => void;
};

export function SyncPullPreviewDialog({
  preview,
  acceptRemoteHead,
  acceptedRemovals,
  busy,
  direction,
  t,
  onAcceptRemovals,
  onApply,
  onClose,
  onResolve,
}: SyncPullPreviewDialogProps) {
  const conflicted = preview.conflicts.length > 0;
  return (
    <ModalShell
      labelledBy="sync-pull-preview-heading"
      onDismiss={() => {
        if (!busy) onClose();
      }}
      placement="sheet"
      panelClassName="max-h-[90vh] w-full max-w-2xl overflow-auto rounded-lg"
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
          {acceptRemoteHead ? (
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
              <ul className="flex flex-col gap-2 text-xs text-notice-ink">
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
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => onResolve("local")}
                  className="rounded border border-line px-3 py-1.5 text-sm text-ink"
                >
                  {t("sync.keepMine")}
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => onResolve("remote")}
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
              <label className="flex items-start gap-2 rounded border border-notice-line bg-notice p-3 text-sm text-notice-ink">
                <input
                  type="checkbox"
                  checked={acceptedRemovals}
                  onChange={(event) => onAcceptRemovals(event.target.checked)}
                  className="mt-0.5"
                />
                <span>{t("sync.confirmOverwrite")}</span>
              </label>
            </>
          )}
          <Button
            kind="primary"
            disabled={
              busy ||
              conflicted ||
              direction === "push" ||
              (preview.removed.length > 0 && !acceptedRemovals)
            }
            onClick={onApply}
            className="self-start"
          >
            {t(acceptRemoteHead ? "sync.remoteHeadApply" : "sync.apply")}
          </Button>
        </div>
    </ModalShell>
  );
}

import type { SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { CheckboxField, hintText, sectionHeading } from "../ui/form";
import { Button, Card, Notice } from "../ui/surface";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { syncRefusals } from "./syncRefusals";

// Where the configured sync stands: last run, the automatic sync switch and
// what stopped it, and the manual actions for this direction.
export function SyncOverviewCard({
  status,
  busy,
  remoteHeadBlocked,
  onToggleAuto,
  onSyncNow,
  onPreviewRemoteHead,
  onPreview,
  onForcePush,
}: {
  status: SyncStatus;
  busy: boolean;
  // The bucket moved on under an automatic pull; the head must be reviewed
  // before anything else runs.
  remoteHeadBlocked: boolean;
  onToggleAuto: (enabled: boolean) => void;
  onSyncNow: () => void;
  onPreviewRemoteHead: () => void;
  onPreview: () => void;
  onForcePush: () => void;
}) {
  const t = useTranslate();
  return (
    <Card as="section" radius="md">
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
        <CheckboxField
            label={t("sync.autoEnable")}
            hint={t(`sync.autoHint.${status.direction}` as MessageKey)}
            checked={status.auto.enabled}
            disabled={busy || !status.keyConfigured}
            onChange={onToggleAuto}
        />
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
                  : (syncRefusals[status.auto.detail ?? ""] ??
                    "sync.autoFailedLast"),
            )}
            code={status.auto.detail ?? "sync_internal_failed"}
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
            onClick={remoteHeadBlocked ? onPreviewRemoteHead : onSyncNow}
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
              onClick={onPreview}
            >
              {t("sync.checkRemoteChanges")}
            </Button>
          )}
          {status.direction === "push" ? null : (
            <Button
              disabled={busy || !status.keyConfigured}
              onClick={onPreviewRemoteHead}
            >
              {t("sync.forcePull")}
            </Button>
          )}
          {status.direction === "pull" ? null : (
            <Button
              kind="danger"
              disabled={busy || !status.keyConfigured}
              onClick={onForcePush}
            >
              {t("sync.forcePushShort")}
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

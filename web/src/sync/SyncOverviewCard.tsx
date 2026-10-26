import type { SyncStatus } from "../api/sync";
import { useLanguage } from "../i18n/context";
import { CheckboxField, hintText } from "../ui/form";
import { PageSection } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import { usePendingAction } from "../ui/usePendingAction";
import { describeLastSync } from "./describeLastSync";
import { SyncErrorNotice } from "./SyncErrorNotice";
import { syncAutoHintKeys, syncDirectionLabelKeys, syncNowLabelKeys } from "./syncMessageKeys";
import { syncRefusals } from "./syncRefusals";

// OverviewAction は、このカードのボタンのうち、押すと engine の応答を待つものである。
// 先頭のボタンは、相手が先へ進んで自動の同期が止まっている間は相手の最新を確かめるボタンに
// 変わるが、押したボタンとしては同じ syncNow に数える。
type OverviewAction = "syncNow" | "checkRemoteChanges" | "forcePull";

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
  onSyncNow: () => Promise<unknown>;
  onPreviewRemoteHead: () => Promise<unknown>;
  onPreview: () => Promise<unknown>;
  onForcePush: () => void;
}) {
  const { locale, t } = useLanguage();
  // busy は、この画面のどこで始めた同期の操作でも立つ。回転の印は、このカードで押したボタン
  // だけに出す。失敗は SyncPanel が文にして出すので、pending.start は断られても reject しない。
  const pending = usePendingAction<OverviewAction>();
  return (
    <PageSection
      heading={t("sync.overviewHeading")}
      hint={describeLastSync(t, status, locale)}
      actions={(
        <span className="rounded-full bg-select-fill px-2.5 py-0.5 text-[11.5px] font-medium text-accent">
          {t(syncDirectionLabelKeys[status.direction])}
        </span>
      )}
    >
      <div className="flex flex-col gap-4">
        <CheckboxField
            label={t("sync.autoEnable")}
            hint={t(syncAutoHintKeys[status.direction])}
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
            busy={pending.running === "syncNow"}
            disabled={busy || !status.keyConfigured}
            onClick={() => void pending.start("syncNow", remoteHeadBlocked ? onPreviewRemoteHead : onSyncNow)}
          >
            {t(
              remoteHeadBlocked
                ? "sync.remoteHeadReview"
                : syncNowLabelKeys[status.direction],
            )}
          </Button>
          {status.direction === "push" || remoteHeadBlocked ? null : (
            <Button
              busy={pending.running === "checkRemoteChanges"}
              disabled={busy || !status.keyConfigured}
              onClick={() => void pending.start("checkRemoteChanges", onPreview)}
            >
              {t("sync.checkRemoteChanges")}
            </Button>
          )}
          {status.direction === "push" ? null : (
            <Button
              busy={pending.running === "forcePull"}
              disabled={busy || !status.keyConfigured}
              onClick={() => void pending.start("forcePull", onPreviewRemoteHead)}
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
    </PageSection>
  );
}

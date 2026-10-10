import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { formatBytes } from "../ui/format";
import type { TransferQueueCounts, TransferQueueSummaryModel } from "./transferQueueSummary";

const countLabelKeys = {
  attention: "sftp.manager.summary.attention",
  failed: "sftp.manager.summary.failed",
  running: "sftp.manager.summary.running",
  reconnecting: "sftp.manager.summary.reconnecting",
  paused: "sftp.manager.summary.paused",
  held: "sftp.manager.summary.held",
  queued: "sftp.manager.summary.queued",
  completed: "sftp.manager.summary.completed",
  cancelled: "sftp.manager.summary.cancelled",
} as const satisfies Record<keyof TransferQueueCounts, MessageKey>;

export function TransferQueueSummary({ summary, id }: { summary: TransferQueueSummaryModel; id?: string }) {
  const t = useTranslate();
  const activeTransferCount = summary.counts.running + summary.counts.reconnecting + summary.counts.paused +
    summary.counts.held + summary.counts.queued + summary.counts.attention;
  const displayedStatuses = (Object.keys(countLabelKeys) as (keyof TransferQueueCounts)[]).filter((status) =>
    summary.counts[status] > 0 && (activeTransferCount === 0 || (status !== "completed" && status !== "cancelled")));
  return (
    <span id={id} className="flex min-w-0 grow flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
      {displayedStatuses.length === 0 ? <span className="text-ink-muted">{t("sftp.manager.summaryIdle", { count: 0 })}</span> : displayedStatuses.map((status) => (
        <span key={status} className={`whitespace-nowrap ${status === "failed" ? "text-danger" : status === "attention" ? "text-notice-ink font-medium" : "text-ink-muted"}`}>
          {t(countLabelKeys[status], { count: summary.counts[status] })}{" "}
        </span>
      ))}
      {summary.counts.running > 0 ? <span className="whitespace-nowrap tabular-nums text-ink-muted">{t("sftp.manager.summaryProgress", { progress: summary.progress, speed: formatBytes(summary.bytesPerSecond) })}</span> : null}
    </span>
  );
}

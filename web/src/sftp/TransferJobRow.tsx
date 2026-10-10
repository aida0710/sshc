import { useId, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Icon, type IconName } from "../ui/icons";
import { formatBytes, formatDuration } from "../ui/format";
import { sftpTransferManager, type ManagedTransferJob } from "./transferManager";
import { getDisplayedTransferStatus, type DisplayedTransferStatus } from "./transferQueueSummaryModel";
import { TransferJobRoute } from "./TransferJobRoute";
import { TransferExclusionSummary } from "./TransferExclusionSummary";
import { sftpTransferProblemText } from "./sftpProblemText";

const statusLabelKeys: Record<DisplayedTransferStatus, MessageKey> = {
  queued: "sftp.manager.status.queued", running: "sftp.manager.status.running",
  reconnecting: "sftp.manager.status.reconnecting", paused: "sftp.manager.status.paused",
  reattach: "sftp.manager.status.reattach", needs_overwrite: "sftp.manager.status.needs_overwrite",
  reconcile: "sftp.manager.status.reconcile", completed: "sftp.manager.status.completed",
  failed: "sftp.manager.status.failed", cancelled: "sftp.manager.status.cancelled",
};

function statusClass(status: DisplayedTransferStatus): string {
  if (status === "failed") return "text-danger";
  if (status === "completed") return "text-live";
  if (status === "needs_overwrite" || status === "reconcile") return "text-notice-ink";
  return "text-ink-muted";
}

function operationIcon(job: ManagedTransferJob): IconName {
  if (job.operation === "delete") return "delete";
  if (job.operation === "put" || job.direction === "upload") return "upload";
  if (job.operation === "get" || job.direction === "download") return "download";
  return "arrowLeftRight";
}

function JobAction({ label, icon, tone = "text-ink-muted", disabled = false, onClick }: {
  label: string; icon: IconName; tone?: string; disabled?: boolean; onClick: () => void;
}) {
  return <button type="button" aria-label={label} title={label} disabled={disabled} onClick={onClick}
    className={`flex size-11 shrink-0 items-center justify-center rounded-md hover:bg-select-fill focus-visible:ring-2 focus-visible:ring-accent disabled:text-ink-faint md:size-8 [@media(pointer:coarse)]:size-11 ${tone}`}>
    <Icon name={icon} className="size-4" />
  </button>;
}

export function TransferJobRow({ job, waiting, processingStopped, maxReconnectAttempts, runControl }: {
  job: ManagedTransferJob;
  waiting: readonly ManagedTransferJob[];
  processingStopped: boolean;
  maxReconnectAttempts: number;
  runControl: (operation: () => Promise<void>) => void;
}) {
  const t = useTranslate();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detailsId = useId();
  const hasSource = sftpTransferManager.hasUploadSource(job.id);
  const displayedStatus = getDisplayedTransferStatus(job, hasSource);
  // The engine also uses reconciliation_required as an in-flight marker;
  // explain it only after the displayed status asks for a destination check.
  const hasProblem = job.problem !== "" && (job.problem !== "sftp_reconciliation_required" || displayedStatus === "reconcile");
  const sourceMissing = job.direction === "upload" && !hasSource &&
    (job.status === "queued" || job.status === "paused" || job.status === "reattach" || job.status === "needs_overwrite");
  const total = Math.max(job.totalBytes >= 0 ? job.totalBytes : job.transferredBytes, 1);
  const capacity = job.operation === "delete" ? `${job.transferredBytes}/${Math.max(job.totalBytes, 0)}`
    : job.totalBytes < 0 ? formatBytes(job.transferredBytes) : `${formatBytes(job.transferredBytes)} / ${formatBytes(job.totalBytes)}`;
  const status = processingStopped && displayedStatus === "queued" ? t("sftp.manager.status.held") : t(statusLabelKeys[displayedStatus]);
  return <li className="min-w-0 border-b border-line/50 last:border-b-0">
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-3 py-2 md:grid-cols-[minmax(10rem,1.3fr)_minmax(8rem,1fr)_auto]">
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <Icon name={operationIcon(job)} className="size-3.5 text-ink-muted" />
          <span className="truncate text-sm font-medium" title={job.name}>{job.name}</span>
        </div>
        <TransferJobRoute job={job} compact />
      </div>
      <div className="col-start-1 row-start-2 min-w-0 space-y-1 text-xs tabular-nums md:col-start-2 md:row-start-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-ink-muted">
          <span>{capacity}</span>
          {job.bytesPerSecond > 0 ? <span>{formatBytes(job.bytesPerSecond)}/s</span> : null}
          {job.remainingSeconds >= 0 && job.status === "running" ? <span>{t("sftp.manager.remaining", { duration: formatDuration(job.remainingSeconds, t) })}</span> : null}
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className={statusClass(displayedStatus)}>{status}</span>
          {job.status === "reconnecting" ? <span className="text-notice-ink">{t("sftp.manager.reconnectAttempt", { attempt: job.reconnectAttempt, maximum: maxReconnectAttempts })}</span> : null}
        </div>
      </div>
      <div className="col-start-2 row-span-2 row-start-1 flex max-w-[8.25rem] flex-wrap items-center justify-end md:col-start-3 md:row-span-1 md:max-w-none [@media(pointer:coarse)]:max-w-[8.25rem]">
        {job.status === "queued" && waiting.length > 1 ? <>
          <JobAction label={t("sftp.manager.moveUp", { name: job.name })} icon="arrowUp" disabled={waiting[0]?.id === job.id} onClick={() => runControl(() => sftpTransferManager.move(job.id, "up"))} />
          <JobAction label={t("sftp.manager.moveDown", { name: job.name })} icon="arrowDown" disabled={waiting[waiting.length - 1]?.id === job.id} onClick={() => runControl(() => sftpTransferManager.move(job.id, "down"))} />
        </> : null}
        {!sourceMissing && job.allowedActions.includes("pause") ? <JobAction label={t("sftp.transfer.pause")} icon="pause" onClick={() => runControl(() => sftpTransferManager.pause(job.id))} /> : null}
        {!sourceMissing && job.allowedActions.includes("resume") && job.status !== "needs_overwrite" ? <JobAction label={t("sftp.transfer.resume")} icon="play" onClick={() => runControl(() => sftpTransferManager.resume(job.id))} /> : null}
        {job.allowedActions.includes("retry") ? <JobAction label={t("sftp.manager.retry")} icon="sync" onClick={() => runControl(() => sftpTransferManager.retry(job.id))} /> : null}
        {!sourceMissing && job.allowedActions.includes("resume") && job.status === "needs_overwrite" ? <JobAction label={t("sftp.overwrite")} icon="upload" tone="text-notice-ink" onClick={() => runControl(() => sftpTransferManager.overwrite(job.id))} /> : null}
        {job.allowedActions.includes("cancel") ? <JobAction label={t("sftp.cancel")} icon="close" tone="text-danger" onClick={() => runControl(() => sftpTransferManager.cancel(job.id))} /> : null}
        {job.allowedActions.includes("remove") ? <JobAction label={t("sftp.manager.remove")} icon="minus" onClick={() => runControl(() => sftpTransferManager.remove(job.id))} /> : null}
        <button type="button" aria-label={t(detailsOpen ? "sftp.manager.hideDetails" : "sftp.manager.showDetails", { name: job.name })}
          aria-expanded={detailsOpen} aria-controls={detailsId} onClick={() => setDetailsOpen((value) => !value)}
          className="flex size-11 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill focus-visible:ring-2 focus-visible:ring-accent md:size-8 [@media(pointer:coarse)]:size-11">
          <Icon name="chevronRight" className={`size-4 ${detailsOpen ? "rotate-90" : ""}`} />
        </button>
      </div>
    </div>
    <progress aria-label={t("sftp.manager.progress", { name: job.name })} className="block h-0.5 w-full accent-accent" max={total} value={job.transferredBytes} />
    {detailsOpen ? <div id={detailsId} className="space-y-2 bg-toolbar/30 px-3 py-3 text-xs">
      <TransferJobRoute job={job} />
      <TransferExclusionSummary patterns={job.excludePatterns ?? []} />
      {hasProblem ? <p className={displayedStatus === "failed" ? "text-danger" : "text-notice-ink"}>{sftpTransferProblemText(t, job.problem)}</p> : null}
    </div> : null}
  </li>;
}

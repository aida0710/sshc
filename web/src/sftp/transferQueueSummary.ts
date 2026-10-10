import type { ManagedTransferJob } from "./transferManager";

export type DisplayedTransferStatus = ManagedTransferJob["status"] | "reconcile";

export type TransferQueueCounts = {
  running: number;
  queued: number;
  held: number;
  paused: number;
  reconnecting: number;
  attention: number;
  failed: number;
  completed: number;
  cancelled: number;
};

// An uncertain publication needs a destination check, even if the engine also
// needs the browser's upload source. Running operations keep their live state.
export function getDisplayedTransferStatus(job: ManagedTransferJob, uploadSourceAvailable: boolean): DisplayedTransferStatus {
  if (job.problem === "sftp_reconciliation_required" && job.status !== "running" && job.status !== "reconnecting") return "reconcile";
  if (job.direction === "upload" && !uploadSourceAvailable &&
    (job.status === "queued" || job.status === "paused" || job.status === "reattach" || job.status === "needs_overwrite")) return "reattach";
  return job.status;
}

export function getTransferQueueSummary(jobs: readonly ManagedTransferJob[], {
  processingStopped,
  hasUploadSource,
}: {
  processingStopped: boolean;
  hasUploadSource: (id: string) => boolean;
}) {
  const counts: TransferQueueCounts = { running: 0, queued: 0, held: 0, paused: 0, reconnecting: 0, attention: 0, failed: 0, completed: 0, cancelled: 0 };
  let totalBytes = 0;
  let transferredBytes = 0;
  let bytesPerSecond = 0;
  for (const job of jobs) {
    const status = getDisplayedTransferStatus(job, hasUploadSource(job.id));
    if (status === "needs_overwrite" || status === "reattach" || status === "reconcile") counts.attention += 1;
    else if (status === "queued" && processingStopped) counts.held += 1;
    else counts[status] += 1;

    if (job.status === "completed" || job.status === "cancelled" || job.status === "failed") continue;
    totalBytes += Math.max(job.totalBytes, 0);
    transferredBytes += Math.max(job.transferredBytes, 0);
    if (job.status === "running") bytesPerSecond += Math.max(job.bytesPerSecond, 0);
  }
  const progress = totalBytes > 0 ? Math.min(100, Math.round((transferredBytes / totalBytes) * 100)) : 0;
  return { counts, totalBytes, transferredBytes, bytesPerSecond, progress };
}

export type TransferQueueSummaryModel = ReturnType<typeof getTransferQueueSummary>;

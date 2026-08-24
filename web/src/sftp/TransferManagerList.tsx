import { useMemo, useSyncExternalStore } from "react";
import { useTranslate } from "../i18n/context";
import { sftpTransferManager, type ManagedTransferJob } from "./transferManager";

function bytes(value: number): string {
  if (value < 1024) return `${Math.max(0, value).toLocaleString()} B`;
  if (value < 1 << 20) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1 << 30) return `${(value / (1 << 20)).toFixed(1)} MiB`;
  return `${(value / (1 << 30)).toFixed(1)} GiB`;
}

function duration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.ceil(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.ceil((seconds % 3600) / 60)}m`;
}

function statusClass(status: ManagedTransferJob["status"]): string {
  if (status === "failed") return "text-danger";
  if (status === "completed") return "text-live";
  if (status === "needs_overwrite") return "text-notice-ink";
  return "text-ink-muted";
}

export function TransferManagerList() {
  const t = useTranslate();
  const jobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const batches = useMemo(() => {
    const grouped = new Map<string, ManagedTransferJob[]>();
    for (const job of jobs) grouped.set(job.batchId, [...(grouped.get(job.batchId) ?? []), job]);
    return [...grouped.entries()];
  }, [jobs]);
  if (jobs.length === 0) return null;
  return (
    <section className="max-h-64 overflow-auto border-b border-line px-3 py-2 text-xs" aria-labelledby="transfer-manager-heading">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <h3 id="transfer-manager-heading" className="font-medium">{t("sftp.manager.heading")}</h3>
        <span className="text-ink-muted">{t("sftp.manager.limit", { count: sftpTransferManager.getMaxConcurrent() })}</span>
        <button type="button" className="ml-auto text-ink-muted hover:text-ink" onClick={() => sftpTransferManager.clearFinished()}>{t("sftp.transfer.clear")}</button>
      </div>
      <div className="space-y-2">
        {batches.map(([batchId, items]) => {
          const first = items[0]!;
          const failed = items.filter((item) => item.status === "failed").length;
          const completed = items.filter((item) => item.status === "completed").length;
          return (
            <section key={batchId} className="rounded-md border border-line bg-surface-subtle p-2" aria-label={first.batchName}>
              <div className="mb-1 flex flex-wrap items-center gap-2">
                <span aria-hidden="true">{first.direction === "upload" ? "↑" : "↓"}</span>
                <span className="min-w-0 grow truncate font-medium" title={first.batchName}>{first.batchName}</span>
                <span className="text-ink-muted">{t(first.batchKind === "folder" ? "sftp.manager.folder" : "sftp.manager.file")}</span>
                <span className="tabular-nums text-ink-muted">{completed}/{items.length}</span>
                {failed > 0 ? <button type="button" className="text-accent" onClick={() => sftpTransferManager.retryFailed(batchId)}>{t("sftp.manager.retryFailed", { count: failed })}</button> : null}
              </div>
              <ul className="space-y-1" aria-label={t("sftp.manager.items")}>
                {items.map((item) => {
                  const total = item.totalBytes >= 0 ? item.totalBytes : Math.max(item.transferredBytes, 1);
                  return (
                    <li key={item.id} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-1">
                      <span className="truncate font-mono" title={`${item.alias}:${item.remotePath}`}>{item.name}</span>
                      <span className="flex items-center justify-self-end gap-1">
                        <progress className="w-14" max={Math.max(total, 1)} value={item.transferredBytes} />
                        <span className="tabular-nums text-ink-muted">{item.totalBytes < 0 ? bytes(item.transferredBytes) : `${bytes(item.transferredBytes)}/${bytes(item.totalBytes)}`}</span>
                      </span>
                      <span className="tabular-nums text-ink-muted">{item.bytesPerSecond > 0 ? `${bytes(item.bytesPerSecond)}/s` : "—"}</span>
                      <span className="tabular-nums text-ink-muted">{item.remainingSeconds >= 0 && item.status === "running" ? t("sftp.manager.remaining", { duration: duration(item.remainingSeconds) }) : "—"}</span>
                      <span className="col-span-2 flex flex-wrap items-center justify-end gap-2 whitespace-nowrap">
                        <span className={statusClass(item.status)}>{item.status === "failed" ? item.problem : t(`sftp.manager.status.${item.status}`)}</span>
                        {(item.status === "running" || item.status === "queued") ? <button type="button" className="text-accent" onClick={() => sftpTransferManager.pause(item.id)}>{t("sftp.transfer.pause")}</button> : null}
                        {(item.status === "paused" || item.status === "reattach") ? <button type="button" className="text-accent" onClick={() => sftpTransferManager.resume(item.id)}>{t("sftp.transfer.resume")}</button> : null}
                        {item.status === "failed" ? <button type="button" className="text-accent" onClick={() => sftpTransferManager.retry(item.id)}>{t("sftp.manager.retry")}</button> : null}
                        {item.status === "needs_overwrite" ? <button type="button" className="text-notice-ink" onClick={() => sftpTransferManager.overwrite(item.id)}>{t("sftp.overwrite")}</button> : null}
                        {!['completed', 'cancelled'].includes(item.status) ? <button type="button" className="text-danger" onClick={() => void sftpTransferManager.cancel(item.id)}>{t("sftp.cancel")}</button> : null}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })}
      </div>
    </section>
  );
}

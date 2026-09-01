import { useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
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
  const [collapsed, setCollapsed] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRoot = useRef<HTMLDivElement>(null);
  const menuPanel = useRef<HTMLDivElement>(null);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const jobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const batches = useMemo(() => {
    const grouped = new Map<string, ManagedTransferJob[]>();
    for (const job of jobs) grouped.set(job.batchId, [...(grouped.get(job.batchId) ?? []), job]);
    return [...grouped.entries()];
  }, [jobs]);
  const canPause = jobs.some((job) => job.allowedActions.includes("pause"));
  const canResume = jobs.some((job) => job.allowedActions.includes("resume") && job.status !== "needs_overwrite");
  const canCancel = jobs.some((job) => job.allowedActions.includes("cancel"));
  const canClear = jobs.some((job) => job.status === "completed" || job.status === "cancelled");
  useDismissibleLayer({
    open: menuOpen,
    containerRefs: [menuRoot],
    onDismiss: () => setMenuOpen(false),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menuOpen, menuRef: menuPanel, onClose: () => setMenuOpen(false) });
  if (jobs.length === 0) return null;
  return (
    <section className="relative shrink-0 border-t border-line bg-toolbar/35 text-xs" aria-labelledby="transfer-manager-heading">
      <div className="flex min-h-9 items-center gap-2 px-3 py-1.5">
        <button type="button" aria-label={t(collapsed ? "sftp.manager.expand" : "sftp.manager.collapse")} aria-expanded={!collapsed} aria-controls="transfer-manager-jobs" onClick={() => setCollapsed((value) => !value)} className="flex min-w-0 items-center gap-1.5 rounded hover:text-accent focus:outline-none focus-visible:ring-1 focus-visible:ring-accent">
          <Icon name="chevronRight" className={`size-3 transition-transform ${collapsed ? "" : "rotate-90"}`} />
          <h3 id="transfer-manager-heading" className="truncate font-medium">{t("sftp.manager.heading")}</h3>
        </button>
        <span className="text-ink-muted">{t("sftp.manager.limit", { count: sftpTransferManager.getMaxConcurrent() })}</span>
        <div ref={menuRoot} className="relative ml-auto">
          <button ref={menuTrigger} type="button" aria-label={t("sftp.manager.actions")} aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)} className="flex size-8 items-center justify-center rounded text-ink-muted hover:bg-select-fill hover:text-ink focus:bg-select-fill focus:outline-none">
            <Icon name="moreHorizontal" className="size-4" />
          </button>
          {menuOpen ? (
            <div ref={menuPanel} role="menu" aria-label={t("sftp.manager.actions")} className="absolute bottom-full right-0 z-20 mb-1 w-48 rounded-lg border border-control-line bg-card p-1 shadow-lg">
              {canPause ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); void sftpTransferManager.pauseAll(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.manager.pauseAll")}</button> : null}
              {canResume ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); void sftpTransferManager.resumeAll(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.manager.resumeAll")}</button> : null}
              {canCancel ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); void sftpTransferManager.cancelAll(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm text-danger hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.manager.cancelAll")}</button> : null}
              {canClear ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); void sftpTransferManager.clearFinished(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.transfer.clear")}</button> : null}
            </div>
          ) : null}
        </div>
      </div>
      {collapsed ? null : <div id="transfer-manager-jobs" className="max-h-56 space-y-2 overflow-auto px-3 pb-2">
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
                {failed > 0 ? <button type="button" className="text-accent" onClick={() => void sftpTransferManager.retryFailed(batchId)}>{t("sftp.manager.retryFailed", { count: failed })}</button> : null}
              </div>
              <ul className="space-y-1" aria-label={t("sftp.manager.items")}>
                {items.map((item) => {
                  const total = item.totalBytes >= 0 ? item.totalBytes : Math.max(item.transferredBytes, 1);
                  const sourceMissing = item.direction === "upload" &&
                    (item.status === "queued" || item.status === "paused" || item.status === "reattach") &&
                    !sftpTransferManager.hasUploadSource(item.id);
                  const displayedStatus: ManagedTransferJob["status"] = sourceMissing ? "reattach" : item.status;
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
                        <span className={statusClass(displayedStatus)}>{displayedStatus === "failed" ? item.problem : t(`sftp.manager.status.${displayedStatus}`)}</span>
                        {!sourceMissing && item.allowedActions.includes("pause") ? <button type="button" className="text-accent" onClick={() => void sftpTransferManager.pause(item.id)}>{t("sftp.transfer.pause")}</button> : null}
                        {!sourceMissing && item.allowedActions.includes("resume") && item.status !== "needs_overwrite" ? <button type="button" className="text-accent" onClick={() => void sftpTransferManager.resume(item.id)}>{t("sftp.transfer.resume")}</button> : null}
                        {item.allowedActions.includes("retry") ? <button type="button" className="text-accent" onClick={() => void sftpTransferManager.retry(item.id)}>{t("sftp.manager.retry")}</button> : null}
                        {item.allowedActions.includes("resume") && item.status === "needs_overwrite" ? <button type="button" className="text-notice-ink" onClick={() => void sftpTransferManager.overwrite(item.id)}>{t("sftp.overwrite")}</button> : null}
                        {item.allowedActions.includes("cancel") ? <button type="button" className="text-danger" onClick={() => void sftpTransferManager.cancel(item.id)}>{t("sftp.cancel")}</button> : null}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })}
      </div>}
    </section>
  );
}

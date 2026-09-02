import { useMemo, useRef, useState, useSyncExternalStore, type PointerEvent as ReactPointerEvent } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { formatBytes as bytes } from "./format";
import { sftpTransferManager, type ManagedTransferJob } from "./transferManager";

function duration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.ceil(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.ceil((seconds % 3600) / 60)}m`;
}

const viewStorageKey = "sshc.sftp.queueView";
const minQueueHeight = 96;
const maxQueueHeight = 560;
const defaultQueueHeight = 224;
const concurrencyChoices = [1, 2, 3, 4, 5, 6, 7, 8];
const autoClearChoices = [0, 30, 300, 3600];

type QueueView = { collapsed: boolean; height: number };

function clampHeight(value: number): number {
  return Math.min(maxQueueHeight, Math.max(minQueueHeight, Math.round(value)));
}

// The queue keeps its size and its folded state across tab switches and
// reloads: it is furniture, not part of any one directory.
function restoreView(): QueueView {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(viewStorageKey) ?? "{}");
    const stored = typeof raw === "object" && raw !== null ? raw as Record<string, unknown> : {};
    return {
      collapsed: stored.collapsed === undefined ? true : stored.collapsed === true,
      height: typeof stored.height === "number" ? clampHeight(stored.height) : defaultQueueHeight,
    };
  } catch {
    return { collapsed: true, height: defaultQueueHeight };
  }
}

function rememberView(view: QueueView): void {
  try {
    window.localStorage.setItem(viewStorageKey, JSON.stringify(view));
  } catch {
    // A browser that refuses storage still keeps the size for this session.
  }
}

function statusClass(status: ManagedTransferJob["status"]): string {
  if (status === "failed") return "text-danger";
  if (status === "completed") return "text-live";
  if (status === "needs_overwrite") return "text-notice-ink";
  return "text-ink-muted";
}

export function TransferManagerList() {
  const t = useTranslate();
  const [view, setView] = useState<QueueView>(restoreView);
  const [menuOpen, setMenuOpen] = useState(false);
  const collapsed = view.collapsed;
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
  const waiting = jobs.filter((job) => job.status === "queued");
  const maxConcurrent = sftpTransferManager.getMaxConcurrent();
  const clearCompletedAfter = sftpTransferManager.getClearCompletedAfter();
  const processingStopped = sftpTransferManager.getProcessingStopped();
  const activeJobs = jobs.filter((job) => job.status !== "completed" && job.status !== "cancelled" && job.status !== "failed");
  const runningJobs = jobs.filter((job) => job.status === "running");
  const aggregateTotal = activeJobs.reduce((sum, job) => sum + Math.max(job.totalBytes, 0), 0);
  const aggregateTransferred = activeJobs.reduce((sum, job) => sum + Math.max(job.transferredBytes, 0), 0);
  const aggregateProgress = aggregateTotal > 0 ? Math.min(100, Math.round((aggregateTransferred / aggregateTotal) * 100)) : 0;
  const aggregateSpeed = runningJobs.reduce((sum, job) => sum + Math.max(job.bytesPerSecond, 0), 0);

  function changeView(next: Partial<QueueView>) {
    setView((current) => {
      const updated = { ...current, ...next };
      rememberView(updated);
      return updated;
    });
  }

  function resizeWithPointer(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    const startY = event.clientY;
    const startHeight = view.height;
    const move = (moveEvent: PointerEvent) => {
      setView((current) => ({ ...current, height: clampHeight(startHeight + (startY - moveEvent.clientY)) }));
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
      setView((current) => {
        rememberView(current);
        return current;
      });
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
    window.addEventListener("pointercancel", stop);
  }

  function resizeWithKeyboard(key: string) {
    if (key === "ArrowUp") changeView({ height: clampHeight(view.height + 32) });
    else if (key === "ArrowDown") changeView({ height: clampHeight(view.height - 32) });
  }

  function applySettings(next: { maxConcurrent?: number; clearCompletedAfterSeconds?: number; processingStopped?: boolean }) {
    void sftpTransferManager.applySettings(
      next.maxConcurrent ?? maxConcurrent,
      next.clearCompletedAfterSeconds ?? clearCompletedAfter,
      next.processingStopped ?? processingStopped,
    ).catch(() => undefined);
  }
  useDismissibleLayer({
    open: menuOpen,
    containerRefs: [menuRoot],
    onDismiss: () => setMenuOpen(false),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menuOpen, menuRef: menuPanel, onClose: () => setMenuOpen(false) });
  if (jobs.length === 0) return null;
  return (
    <section className="relative mt-2 shrink-0 overflow-hidden rounded-md border border-line/60 bg-toolbar/30 text-xs" aria-labelledby="transfer-manager-heading">
      {collapsed ? null : (
        <div
          role="separator"
          aria-orientation="horizontal"
          aria-label={t("sftp.manager.resize")}
          aria-valuenow={view.height}
          aria-valuemin={minQueueHeight}
          aria-valuemax={maxQueueHeight}
          tabIndex={0}
          onPointerDown={resizeWithPointer}
          onKeyDown={(event) => {
            if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
            event.preventDefault();
            resizeWithKeyboard(event.key);
          }}
          className="absolute inset-x-0 -top-1 z-10 h-2 cursor-row-resize focus:outline-none focus-visible:bg-accent/40"
        />
      )}
      <div className="flex min-h-9 items-center gap-2 px-3 py-1.5 md:min-h-8 md:py-1">
        <button type="button" aria-label={t(collapsed ? "sftp.manager.expand" : "sftp.manager.collapse")} aria-expanded={!collapsed} aria-controls="transfer-manager-jobs" onClick={() => changeView({ collapsed: !collapsed })} className="flex min-w-0 items-center gap-1.5 rounded hover:text-accent focus:outline-none focus-visible:ring-1 focus-visible:ring-accent">
          <Icon name="chevronRight" className={`size-3 transition-transform ${collapsed ? "" : "rotate-90"}`} />
          <h3 id="transfer-manager-heading" className={`${collapsed ? "text-ink-muted" : "text-ink"} truncate font-medium`}>{t("sftp.manager.heading")}</h3>
        </button>
        {collapsed ? (
          <>
            <span className="min-w-0 grow truncate font-medium text-ink">
              {activeJobs.length > 0
                ? t("sftp.manager.summaryRunning", { count: activeJobs.length, progress: aggregateProgress, speed: bytes(aggregateSpeed) })
                : t("sftp.manager.summaryIdle", { count: jobs.length })}
            </span>
            {aggregateTotal > 0 ? <progress className="hidden w-28 sm:block" max={aggregateTotal} value={aggregateTransferred} /> : null}
          </>
        ) : (
        <>
          <button
          type="button"
          aria-pressed={processingStopped}
          aria-label={t(processingStopped ? "sftp.manager.startProcessing" : "sftp.manager.stopProcessing")}
          onClick={() => applySettings({ processingStopped: !processingStopped })}
          className={`flex size-9 items-center justify-center rounded md:size-7 ${processingStopped ? "text-notice-ink" : "text-ink-muted"} hover:bg-select-fill focus:bg-select-fill focus:outline-none`}
        >
          <span aria-hidden="true">{processingStopped ? "▶" : "⏸"}</span>
        </button>
        <label className="flex items-center gap-1 text-ink-muted">
          <span className="hidden sm:inline">{t("sftp.manager.concurrency")}</span>
          <select
            aria-label={t("sftp.manager.concurrency")}
            value={maxConcurrent}
            onChange={(event) => applySettings({ maxConcurrent: Number(event.target.value) })}
            className="rounded border border-control-line bg-control px-1 py-0.5 text-xs"
          >
            {concurrencyChoices.map((choice) => <option key={choice} value={choice}>{choice}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-1 text-ink-muted">
          <span className="hidden md:inline">{t("sftp.manager.autoClear")}</span>
          <select
            aria-label={t("sftp.manager.autoClear")}
            value={autoClearChoices.includes(clearCompletedAfter) ? clearCompletedAfter : 0}
            onChange={(event) => applySettings({ clearCompletedAfterSeconds: Number(event.target.value) })}
            className="rounded border border-control-line bg-control px-1 py-0.5 text-xs"
          >
            {autoClearChoices.map((choice) => (
              <option key={choice} value={choice}>{choice === 0 ? t("sftp.manager.autoClearOff") : duration(choice)}</option>
            ))}
          </select>
        </label>
        </>
        )}
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
      {collapsed ? null : <div id="transfer-manager-jobs" style={{ height: view.height }} className="space-y-1.5 overflow-auto px-2.5 pb-2.5">
        {batches.map(([batchId, items]) => {
          const first = items[0]!;
          const failed = items.filter((item) => item.status === "failed").length;
          const completed = items.filter((item) => item.status === "completed").length;
          return (
            <section key={batchId} className="rounded-md bg-surface-subtle/70 p-2" aria-label={first.batchName}>
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
                        <span className={statusClass(displayedStatus)}>
                          {displayedStatus === "failed"
                            ? item.problem
                            : processingStopped && displayedStatus === "queued"
                              ? t("sftp.manager.status.held")
                              : t(`sftp.manager.status.${displayedStatus}`)}
                        </span>
                        {item.status === "queued" && waiting.length > 1 ? (
                          <>
                            <button type="button" aria-label={t("sftp.manager.moveUp", { name: item.name })} disabled={waiting[0]?.id === item.id} onClick={() => void sftpTransferManager.move(item.id, "up")} className="flex size-9 items-center justify-center rounded text-accent disabled:text-ink-faint md:size-5">↑</button>
                            <button type="button" aria-label={t("sftp.manager.moveDown", { name: item.name })} disabled={waiting[waiting.length - 1]?.id === item.id} onClick={() => void sftpTransferManager.move(item.id, "down")} className="flex size-9 items-center justify-center rounded text-accent disabled:text-ink-faint md:size-5">↓</button>
                          </>
                        ) : null}
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

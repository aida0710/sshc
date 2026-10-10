import { TransferBatchList } from "./TransferBatchList";
import { TransferSettingsDialog } from "./TransferSettingsDialog";
import { useEffect, useId, useMemo, useRef, useState, useSyncExternalStore, type PointerEvent as ReactPointerEvent } from "react";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { Notice } from "../ui/surface";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { readStoredJSON, writeStoredJSON } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { sftpTransferManager, type ManagedTransferJob } from "./transferManager";
import type { TransferSettings } from "./api";
import { getTransferQueueSummary } from "./transferQueueSummary";
import { TransferQueueSummary } from "./TransferQueueSummary";

const minQueueHeight = 96;
const maxQueueHeight = 560;
const defaultQueueHeight = 224;
// An arrow key moves the divider by this much, so about fifteen presses cross
// the whole range from minQueueHeight to maxQueueHeight.
const keyboardResizeStep = 32;
type QueueView = { collapsed: boolean; height: number };

function clampHeight(value: number): number {
  return Math.min(maxQueueHeight, Math.max(minQueueHeight, Math.round(value)));
}

// Desktop size and folded state persist across directories. On mobile, the
// dock always remains compact and its details open in a separate sheet.
function restoreView(): QueueView {
  const raw = readStoredJSON(localStorageKeys.sftpQueueView, {});
  const stored = typeof raw === "object" && raw !== null ? raw as Record<string, unknown> : {};
  return {
    collapsed: stored.collapsed === undefined ? true : stored.collapsed === true,
    height: typeof stored.height === "number" ? clampHeight(stored.height) : defaultQueueHeight,
  };
}

function rememberView(view: QueueView): void {
  writeStoredJSON(localStorageKeys.sftpQueueView, view);
}

export function TransferManagerList({ openRequest = 0 }: { openRequest?: number }) {
  const t = useTranslate();
  const [view, setView] = useState<QueueView>(restoreView);
  const compactViewport = useMediaQuery(mobileViewportQuery);
  const [sheetOpen, setSheetOpen] = useState(false);
  const handledOpenRequest = useRef(0);
  useEffect(() => {
    if (openRequest === 0 || handledOpenRequest.current === openRequest) return;
    handledOpenRequest.current = openRequest;
    setView((current) => {
      const updated = { ...current, collapsed: false };
      rememberView(updated);
      return updated;
    });
    if (compactViewport) setSheetOpen(true);
  }, [openRequest, compactViewport]);
  const dockTrigger = useRef<HTMLButtonElement>(null);
  const closeSheet = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const [menuOpen, setMenuOpen] = useState(false);
  const [controlProblem, setControlProblem] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsTrigger = useRef<HTMLButtonElement>(null);
  const collapsed = compactViewport ? !sheetOpen : view.collapsed;
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
  const canClear = jobs.some((job) => (job.status === "completed" || job.status === "cancelled") && job.allowedActions.includes("remove"));
  const canClearFailed = jobs.some((job) => job.status === "failed" && job.allowedActions.includes("remove"));
  const hasMenuActions = canPause || canResume || canCancel || canClear || canClearFailed;
  const waiting = jobs.filter((job) => job.status === "queued");
  const maxConcurrent = sftpTransferManager.getMaxConcurrent();
  const clearCompletedAfter = sftpTransferManager.getClearCompletedAfter();
  const processingStopped = sftpTransferManager.getProcessingStopped();
  const largeFileThreshold = sftpTransferManager.getLargeFileThreshold();
  const largeFileParallelism = sftpTransferManager.getLargeFileParallelism();
  const largeFileChunkBytes = sftpTransferManager.getLargeFileChunkBytes();
  const summary = getTransferQueueSummary(jobs, { processingStopped, hasUploadSource: (id) => sftpTransferManager.hasUploadSource(id) });
  const queueHeight = view.height;
  const queueMaximum = maxQueueHeight;

  function dismissSheet() {
    setMenuOpen(false);
    setSheetOpen(false);
  }

  function changeView(next: Partial<QueueView>) {
    setView((current) => {
      const updated = { ...current, ...next };
      rememberView(updated);
      return updated;
    });
  }

  function changeQueueHeight(height: number, remember = false) {
    setView((current) => {
      const updated = { ...current, height };
      if (remember) rememberView(updated);
      return updated;
    });
  }

  function resizeWithPointer(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    const startY = event.clientY;
    const startHeight = queueHeight;
    const move = (moveEvent: PointerEvent) => {
      const nextHeight = Math.min(queueMaximum, Math.max(minQueueHeight, Math.round(startHeight + (startY - moveEvent.clientY))));
      changeQueueHeight(nextHeight);
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
    if (key === "Home") changeQueueHeight(minQueueHeight, true);
    else if (key === "End") changeQueueHeight(maxQueueHeight, true);
    else if (key === "ArrowUp") changeQueueHeight(clampHeight(queueHeight + keyboardResizeStep), true);
    else if (key === "ArrowDown") changeQueueHeight(clampHeight(queueHeight - keyboardResizeStep), true);
  }

  const currentSettings: TransferSettings = {
    maxConcurrent,
    clearCompletedAfterSeconds: clearCompletedAfter,
    processingStopped,
    largeFileThresholdBytes: largeFileThreshold,
    largeFileParallelism,
    largeFileChunkBytes,
    speedLimitBytesPerSecond: sftpTransferManager.getSpeedLimitBytesPerSecond(),
    autoReconnect: sftpTransferManager.getAutoReconnect(),
    maxReconnectAttempts: sftpTransferManager.getMaxReconnectAttempts(),
    excludePatterns: [...sftpTransferManager.getExcludePatterns()],
  };

  function applySettings(next: Partial<TransferSettings>) {
    runControl(() => sftpTransferManager.applySettings({ ...currentSettings, ...next }));
  }

  function runControl(operation: () => Promise<void>) {
    setControlProblem("");
    void operation().catch(async (error) => {
      await sftpTransferManager.reconcile().catch(() => undefined);
      const code = failureCode(error) || (error instanceof Error ? error.message : "");
      setControlProblem(code === "sftp_transfer_state"
        ? t("sftp.manager.controlChanged")
        : code === "sftp_failed" || code === "sftp_cleanup_pending"
          ? t("sftp.manager.cleanupFailed")
          : t("sftp.manager.controlFailed"));
    });
  }
  useDismissibleLayer({
    open: menuOpen,
    containerRefs: [menuRoot],
    onDismiss: () => setMenuOpen(false),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menuOpen, menuRef: menuPanel, onClose: () => setMenuOpen(false) });
  const content = (
    <section className={compactViewport ? "flex min-h-0 flex-1 flex-col text-sm" : "relative mt-3 shrink-0 overflow-visible rounded-md border border-line/60 bg-toolbar/30 text-xs md:mt-2"} aria-labelledby={headingId}>
      {compactViewport || collapsed || jobs.length === 0 ? null : (
        <div
          role="separator"
          aria-orientation="horizontal"
          aria-label={t("sftp.manager.resize")}
          aria-valuenow={queueHeight}
          aria-valuemin={minQueueHeight}
          aria-valuemax={queueMaximum}
          tabIndex={0}
          onPointerDown={resizeWithPointer}
          onKeyDown={(event) => {
            if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
            event.preventDefault();
            resizeWithKeyboard(event.key);
          }}
          className="group absolute inset-x-0 -top-3 z-10 flex h-6 touch-none cursor-row-resize items-center justify-center focus:outline-none"
        >
          <span aria-hidden="true" className="h-1 w-10 rounded-full bg-control-line transition-colors group-hover:bg-ink-muted group-active:bg-accent group-focus-visible:bg-accent" />
        </div>
      )}
      <div className={`relative flex min-w-0 shrink-0 items-center gap-2 px-3 ${compactViewport ? "min-h-14 flex-wrap border-b border-line py-1" : "min-h-9 py-1.5 md:min-h-8 md:py-1"}`}>
        {compactViewport ? <h3 id={headingId} className="min-w-0 flex-1 truncate font-medium">{t("sftp.manager.heading")}</h3> : (
        <button type="button" aria-label={t(collapsed ? "sftp.manager.expand" : "sftp.manager.collapse")} aria-describedby={`${headingId}-summary`} aria-expanded={!collapsed} aria-controls={`${headingId}-jobs`} onClick={() => changeView({ collapsed: !collapsed })} className="flex min-w-0 flex-1 items-center gap-2 rounded text-left hover:text-accent focus:outline-none focus-visible:ring-1 focus-visible:ring-accent">
          <DisclosureChevron expanded={!collapsed} className="size-3" />
          <h3 id={headingId} className={`${collapsed ? "text-ink-muted" : "text-ink"} shrink-0 truncate font-medium`}>{t("sftp.manager.heading")}</h3>
          <TransferQueueSummary summary={summary} id={`${headingId}-summary`} compact />
        </button>
        )}
        {compactViewport ? <div className="order-last w-full pb-2">
          <TransferQueueSummary summary={summary} id={`${headingId}-summary`} compact />
        </div> : null}
        {collapsed && summary.totalBytes > 0 ? <progress className="hidden w-28 sm:block" max={summary.totalBytes} value={summary.transferredBytes} /> : null}
        {collapsed ? null : (
        <>
          <button
          type="button"
          aria-pressed={processingStopped}
          aria-label={t(processingStopped ? "sftp.manager.startProcessing" : "sftp.manager.stopProcessing")}
          onClick={() => applySettings({ processingStopped: !processingStopped })}
          className={`flex size-11 shrink-0 items-center justify-center rounded md:size-8 [@media(pointer:coarse)]:size-11 ${processingStopped ? "text-notice-ink" : "text-ink-muted"} hover:bg-select-fill focus:bg-select-fill focus:outline-none`}
        >
          <Icon name={processingStopped ? "play" : "pause"} className="size-4" />
        </button>
        </>
        )}
        <button ref={settingsTrigger} type="button" aria-label={t("sftp.manager.settings")} title={t("sftp.manager.settings")} onClick={() => setSettingsOpen(true)} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill md:size-8 [@media(pointer:coarse)]:size-11"><Icon name="settings" className="size-4" /></button>
        <div ref={menuRoot} className="relative ml-auto">
          {hasMenuActions ? <button ref={menuTrigger} type="button" aria-label={t("sftp.manager.actions")} aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)} className="flex size-11 items-center justify-center rounded md:size-8 [@media(pointer:coarse)]:size-11 text-ink-muted hover:bg-select-fill hover:text-ink focus:bg-select-fill focus:outline-none">
            <Icon name="moreHorizontal" className="size-4" />
          </button> : null}
          {menuOpen ? (
            <div ref={menuPanel} role="menu" aria-label={t("sftp.manager.actions")} className={`absolute right-0 z-20 w-48 rounded-lg border border-control-line bg-card p-1 shadow-lg ${compactViewport ? "top-full mt-1" : "bottom-full mb-1"}`}>
              {canPause ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); runControl(() => sftpTransferManager.pauseAll()); }} className="block min-h-11 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-8 [@media(pointer:coarse)]:min-h-11">{t("sftp.manager.pauseAll")}</button> : null}
              {canResume ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); runControl(() => sftpTransferManager.resumeAll()); }} className="block min-h-11 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-8 [@media(pointer:coarse)]:min-h-11">{t("sftp.manager.resumeAll")}</button> : null}
              {canCancel ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); runControl(() => sftpTransferManager.cancelAll()); }} className="block min-h-11 w-full rounded px-2.5 py-2 text-left text-sm text-danger hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-8 [@media(pointer:coarse)]:min-h-11">{t("sftp.manager.cancelAll")}</button> : null}
              {canClear ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); runControl(() => sftpTransferManager.clearFinished()); }} className="block min-h-11 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-8 [@media(pointer:coarse)]:min-h-11">{t("sftp.transfer.clear")}</button> : null}
              {canClearFailed ? <button type="button" role="menuitem" onClick={() => { setMenuOpen(false); runControl(() => sftpTransferManager.clearFailed()); }} className="block min-h-11 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-8 [@media(pointer:coarse)]:min-h-11">{t("sftp.manager.clearFailed")}</button> : null}
            </div>
          ) : null}
        </div>
        {compactViewport ? <button ref={closeSheet} type="button" aria-label={t("sftp.manager.close")} onClick={dismissSheet} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill"><Icon name="close" className="size-4" /></button> : null}
      </div>
      {controlProblem !== "" ? <div className="mx-2.5 mb-2"><Notice tone="danger" compact><span className="grow">{controlProblem}</span><button type="button" aria-label={t("sftp.manager.dismissError")} onClick={() => setControlProblem("")} className="shrink-0 text-ink-muted hover:text-ink"><Icon name="close" className="size-3.5" /></button></Notice></div> : null}
      {compactViewport && jobs.length === 0 ? <p className="p-6 text-center text-ink-muted">{t("sftp.manager.summaryIdle", { count: 0 })}</p> : null}
      {collapsed || jobs.length === 0 ? null : <div id={`${headingId}-jobs`} style={compactViewport ? undefined : { height: queueHeight }} className={`space-y-1.5 overflow-auto overscroll-contain px-2.5 pb-2.5 ${compactViewport ? "min-h-0 flex-1 pt-2" : ""}`}>
        <TransferBatchList batches={batches} waiting={waiting} processingStopped={processingStopped}
          maxReconnectAttempts={sftpTransferManager.getMaxReconnectAttempts()} runControl={runControl} />
      </div>}
    </section>
  );
  const settingsDialog = <TransferSettingsDialog open={settingsOpen} settings={currentSettings} problem={controlProblem}
    onCommit={applySettings} onDismiss={() => setSettingsOpen(false)} returnFocusRef={settingsTrigger} />;
  if (!compactViewport) return <>{content}{settingsDialog}</>;
  return (
    <>
      <button
        ref={dockTrigger}
        type="button"
        aria-label={t("sftp.manager.expand")}
        aria-describedby={`${headingId}-dock-summary`}
        aria-haspopup="dialog"
        aria-expanded={sheetOpen}
        onClick={() => setSheetOpen(true)}
        className="relative mt-1 flex min-h-11 shrink-0 items-center gap-2 rounded-md border border-line/60 bg-toolbar/50 px-3 py-1 text-left text-xs active:bg-select-fill"
      >
        <Icon name="chevronRight" className="size-3 -rotate-90 text-ink-muted" />
        <span className="sr-only">{t("sftp.manager.heading")}</span>
        <TransferQueueSummary summary={summary} id={`${headingId}-dock-summary`} compact />
        {summary.totalBytes > 0 ? <span aria-hidden="true" className="absolute bottom-0 left-0 h-0.5 rounded-full bg-accent transition-[width]" style={{ width: `${summary.progress}%` }} /> : null}
      </button>
      <ModalShell open={sheetOpen} labelledBy={headingId} onDismiss={dismissSheet} closeOnOutside initialFocusRef={closeSheet} returnFocusRef={dockTrigger} placement="sheet" panelClassName="flex h-[min(36rem,85dvh)] max-h-full w-full max-w-2xl flex-col overflow-hidden rounded-xl">
        {content}
      </ModalShell>
      {settingsDialog}
    </>
  );
}

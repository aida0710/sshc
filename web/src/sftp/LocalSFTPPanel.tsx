import { useCallback, useEffect, useId, useRef, useState, type DragEvent } from "react";
import { failureCode } from "../api/client";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import { nextSort } from "../ui/tableSort";
import { mobileViewportQuery, useCompactViewport, useMediaQuery } from "../ui/useMediaQuery";
import { sftpApi, type LocalListing } from "./api";
import { formatBytes } from "./format";
import { SFTPEntryList, sortEntries, useSFTPEntryList, type SFTPSort, type SFTPSortState } from "./SFTPEntryList";
import { SFTPHostPicker } from "./SFTPHostPicker";
import { SFTPNavigationControls } from "./SFTPNavigationControls";
import { localHostAlias } from "./localHost";
import { remoteEntriesMime, type RemoteDragPayload } from "./transfers";
import { sftpTransferManager } from "./transferManager";

function localRoot(value: string): string {
  const unc = /^\/\/[^/]+\/[^/]+(?:\/|$)/.exec(value);
  if (unc) return `${unc[0].replace(/\/$/, "")}/`;
  return /^[A-Za-z]:\//.test(value) ? value.slice(0, 3) : "/";
}
function parentPath(value: string): string {
  const normalized = value.replace(/\\/g, "/");
  const root = localRoot(normalized);
  if (normalized === root || normalized === root.replace(/\/$/, "")) return value;
  const withoutTrailingSlash = normalized.replace(/\/$/, "");
  const index = withoutTrailingSlash.lastIndexOf("/");
  if (index < root.length) return root;
  return withoutTrailingSlash.slice(0, index);
}
function joinPath(parent: string, name: string): string {
  return `${parent.replace(/\/$/, "")}/${name}`;
}
function crumbs(value: string): { label: string; path: string }[] {
  const normalized = value.replace(/\\/g, "/");
  const root = localRoot(normalized);
  if (normalized === root) return [{ label: root, path: root }];
  const parts = normalized.slice(root.length).split("/").filter(Boolean);
  return [{ label: root, path: root }, ...parts.map((part, index) => ({ label: part, path: `${root}${parts.slice(0, index + 1).join("/")}` }))];
}

const noEntries: LocalListing["entries"] = [];

export function LocalSFTPPanel({
  aliases, hosts, initialPath, initialSort = { key: "name", direction: "ascending" }, remote,
  onHostChange, onQueueOpen, onDirectoryChange, onSortChange = () => undefined,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  initialPath: string;
  initialSort?: SFTPSortState;
  remote: { alias: string; path: string } | null;
  onHostChange: (alias: string) => void;
  onQueueOpen: () => void;
  onDirectoryChange: (path: string | null) => void;
  onSortChange?: (sort: SFTPSortState) => void;
}) {
  const t = useTranslate();
  const actionsHeadingId = useId();
  const panelRoot = useRef<HTMLElement>(null);
  const compactViewport = useCompactViewport(panelRoot);
  const mobileInteraction = useMediaQuery(mobileViewportQuery);
  const [listing, setListing] = useState<LocalListing | null>(null);
  const [busy, setBusy] = useState(false);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const [problem, setProblem] = useState("");
  const [dragging, setDragging] = useState(false);
  const [pathEditing, setPathEditing] = useState(false);
  const [pathDraft, setPathDraft] = useState("");
  const [history, setHistory] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const [mobileActionsOpen, setMobileActionsOpen] = useState(false);
  const [sort, setSort] = useState<SFTPSortState>(initialSort);
  const currentPath = useRef("");
  const requestedPath = useRef(initialPath);
  const loadGeneration = useRef(0);
  const startingPath = useRef(initialPath);
  const reportDirectory = useRef(onDirectoryChange);
  reportDirectory.current = onDirectoryChange;
  const completed = useRef(new Set(sftpTransferManager.getSnapshot()
    .filter((job) => job.direction === "remote" && job.status === "completed").map((job) => job.id)));

  const loadedEntries = listing?.entries ?? noEntries;
  const displayedEntries = sortEntries(loadedEntries, sort);
  const parentRowVisible = listing !== null && parentPath(listing.path) !== listing.path;
  const list = useSFTPEntryList({
    entries: displayedEntries,
    loadedEntries,
    parentRowVisible,
    busy,
    mobileInteraction,
    onActivate: (entry) => { if (entry.type === "directory") void navigate(entry.path); },
    onOpenParent: () => { if (listing !== null) void navigate(parentPath(listing.path)); },
  });
  const { setSelectedPaths, selectedEntries, selectedEntry, openParent } = list;
  // The listing failed before anything could be shown, so the rows' place
  // says so with the retry; a failure while a directory is showing is a banner.
  const listingFailed = problem !== "" && listing === null;

  function changeSort(key: SFTPSort) {
    setSort((current) => {
      const next = nextSort(current.key, current.direction, key);
      onSortChange(next);
      return next;
    });
  }
  function editPath() {
    if (listing === null) return;
    setPathDraft(listing.path);
    setPathEditing(true);
  }
  async function copyPath() {
    if (listing === null) return;
    try { await clipboard.writeText(listing.path); setProblem(""); }
    catch { setProblem(t("copy.refused")); }
  }

  const navigate = useCallback(async (path: string, recordHistory = true) => {
    const generation = ++loadGeneration.current;
    requestedPath.current = path;
    setBusy(true);
    setPendingPath(path);
    try {
      const next = await sftpApi.listLocal(path);
      if (generation !== loadGeneration.current) return;
      currentPath.current = next.path;
      setListing(next);
      setSelectedPaths((current) => new Set(next.entries.filter((entry) => current.has(entry.path)).map((entry) => entry.path)));
      reportDirectory.current(next.path);
      setProblem("");
      if (recordHistory) setHistory((current) => {
        if (current.paths[current.index] === next.path) return current;
        const paths = [...current.paths.slice(0, current.index + 1), next.path];
        return { paths, index: paths.length - 1 };
      });
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      if (generation === loadGeneration.current) { setBusy(false); setPendingPath(null); }
    }
  }, [setSelectedPaths]);

  async function navigateHistory(delta: number) {
    const index = history.index + delta;
    const path = history.paths[index];
    if (path === undefined) return;
    await navigate(path, false);
    if (currentPath.current === path) setHistory((current) => ({ ...current, index }));
  }

  useEffect(() => { void navigate(startingPath.current); }, [navigate]);
  useEffect(() => sftpTransferManager.subscribe(() => {
    for (const job of sftpTransferManager.getSnapshot()) {
      if (job.direction !== "remote" || job.operation !== "get" ||
          job.status !== "completed" || completed.current.has(job.id)) continue;
      completed.current.add(job.id);
      if (currentPath.current !== "") void navigate(currentPath.current, false);
    }
  }), [navigate]);

  async function upload() {
    if (listing === null || remote === null || !remote.alias || !remote.path) return;
    if (selectedEntries.length === 0) return;
    setBusy(true);
    try {
      await sftpTransferManager.addRemoteTransfers(selectedEntries.map((entry) => ({
        sourceAlias: remote.alias, sourcePath: entry.path,
        targetAlias: remote.alias, targetPath: joinPath(remote.path, entry.name),
        name: entry.name, kind: entry.type === "directory" ? "folder" : "file", totalBytes: entry.size,
      })), "put");
      onQueueOpen();
      setProblem("");
    } catch (error) { setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed")); }
    finally { setBusy(false); }
  }
  async function acceptRemoteDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault(); setDragging(false);
    if (listing === null || busy) return;
    try {
      const payload = JSON.parse(event.dataTransfer.getData(remoteEntriesMime)) as RemoteDragPayload;
      if (typeof payload.alias !== "string" || !Array.isArray(payload.entries) || payload.entries.some((entry) =>
        typeof entry.path !== "string" || (entry.type !== "file" && entry.type !== "directory"))) throw new Error("sftp_local_drop_invalid");
      await sftpTransferManager.addRemoteTransfers(payload.entries.map((entry) => ({
        sourceAlias: payload.alias, sourcePath: entry.path,
        targetAlias: payload.alias, targetPath: joinPath(listing.path, entry.path.split("/").at(-1) ?? ""),
        name: entry.path.split("/").at(-1) ?? "", kind: entry.type === "directory" ? "folder" : "file", totalBytes: entry.size,
      })), "get");
      onQueueOpen();
    } catch (error) { setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_local_drop_invalid")); }
  }
  const canUpload = selectedEntries.length > 0 && remote !== null && remote.alias !== "" && remote.path !== "";
  return <section ref={panelRoot} className="flex h-full min-h-0 min-w-0 flex-col gap-1.5 md:gap-1" aria-label={t("sftp.local.heading")}>
    <div className="flex min-h-10 shrink-0 items-center gap-1.5 border-b border-line/50 pb-1.5 md:pb-1">
      <SFTPHostPicker aliases={aliases} {...(hosts === undefined ? {} : { hosts })} value={localHostAlias}
        onChange={onHostChange} compact={mobileInteraction} includeLocal />
      {mobileInteraction ? <button type="button" aria-label={t("sftp.back")}
        disabled={busy || history.index <= 0} onClick={() => void navigateHistory(-1)}
        className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint">←</button>
        : <SFTPNavigationControls busy={busy} canBack={history.index > 0}
          canForward={history.index >= 0 && history.index < history.paths.length - 1}
          canHome={listing !== null} canRoot={listing !== null && listing.path !== localRoot(listing.path)}
          onBack={() => void navigateHistory(-1)} onForward={() => void navigateHistory(1)}
          onHome={() => void navigate("")} onRoot={() => { if (listing !== null) void navigate(localRoot(listing.path)); }} />}
      {listing === null ? <span className="min-w-0 flex-1" /> : mobileInteraction ? (
        pathEditing ? <input autoFocus aria-label={t("sftp.local.pathInput")} value={pathDraft}
          onChange={(event) => setPathDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") setPathEditing(false);
            if (event.key === "Enter") { setPathEditing(false); void navigate(pathDraft.trim()); }
          }} className="h-11 min-w-0 flex-1 rounded border border-control-line bg-control px-2 font-mono text-base" />
          : <button type="button" aria-label={t("sftp.local.editPath")} title={listing.path} onClick={editPath}
              className="flex h-11 min-w-0 flex-1 items-center gap-1 rounded px-2 text-left active:bg-select-fill">
            <span className="truncate font-mono text-sm font-medium">{listing.path === listing.home ? "~" : crumbs(listing.path).at(-1)?.label}</span>
            <Icon name="chevronRight" className="size-3 shrink-0 rotate-90 text-ink-muted" />
          </button>
      ) : (
      <nav aria-label={t("sftp.local.path")} className="flex min-h-9 min-w-0 flex-1 items-center gap-1 overflow-hidden rounded-md bg-control/60 px-1">
        {pathEditing ? <input autoFocus aria-label={t("sftp.local.pathInput")} value={pathDraft}
          onChange={(event) => setPathDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") setPathEditing(false);
            if (event.key === "Enter") { setPathEditing(false); void navigate(pathDraft.trim()); }
          }} className="min-w-0 flex-1 rounded border border-line bg-surface px-2 py-1 text-sm" />
          : <div data-testid="sftp-local-path-space" onClick={(event) => { if (event.target === event.currentTarget && !busy) editPath(); }}
              title={t("sftp.local.editPath")}
              className="flex min-w-0 flex-1 cursor-text items-center overflow-x-auto whitespace-nowrap text-sm">
          {crumbs(listing.path).map((crumb, index) => <span key={crumb.path} className="inline-flex items-center">
            {index > 0 ? <Icon name="chevronRight" className="mx-1 size-3 text-ink-faint" /> : null}
            {crumb.path === listing.path ? <span className="px-1 font-medium" aria-current="location">{crumb.path === listing.home ? "~" : crumb.label}</span>
              : <button type="button" onClick={() => { void navigate(crumb.path); }} disabled={busy}
                  className="rounded px-1 py-1 text-ink-muted hover:bg-hover hover:text-ink">{crumb.path === listing.home ? "~" : crumb.label}</button>}
          </span>)}
        </div>}
        <button type="button" aria-label={t("sftp.copyPath")} title={t("sftp.copyPath")}
          onClick={() => { void copyPath(); }} className="rounded p-2 hover:bg-hover"><Icon name="copy" className="size-3.5" /></button>
        <button type="button" aria-label={t("sftp.local.editPath")} title={t("sftp.local.editPath")}
          onClick={() => { if (pathEditing) setPathEditing(false); else editPath(); }}
          className="rounded p-2 hover:bg-hover"><Icon name="edit" className="size-3.5" /></button>
      </nav>
      )}
      <button type="button" className="flex size-11 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-8" aria-label={t("sftp.refreshDirectory")}
        disabled={listing === null || busy} onClick={() => { void navigate(listing?.path ?? "", false); }}><Icon name="sync" className="size-4" /></button>
      {mobileInteraction ? <button type="button" aria-label={t("sftp.mobile.actions")} onClick={() => setMobileActionsOpen(true)}
        className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="moreHorizontal" className="size-4" /></button> : null}
    </div>
    {problem === "" || listingFailed ? null : <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}
    <div
      aria-label={t("sftp.local.dropZone")}
      aria-busy={busy}
      className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-md border bg-card transition-shadow ${dragging ? "border-accent ring-1 ring-accent" : "border-line/60"}`}
      onDragOver={(event) => { if (event.dataTransfer.types.includes(remoteEntriesMime)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; setDragging(true); } }}
      onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
      onDrop={(event) => { void acceptRemoteDrop(event); }}
    >
      <div className={`${mobileInteraction && selectedEntries.length === 0 ? "hidden" : "flex"} min-h-10 shrink-0 items-center gap-1 border-b border-line/50 bg-toolbar/45 px-2 py-1 md:min-h-8 md:py-0.5`}>
        {selectedEntries.length > 0 ? <>
          <button type="button" aria-label={t("sftp.clearSelection")} onClick={() => setSelectedPaths(new Set())} className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink md:size-7">
            <Icon name="close" className="size-3.5" />
          </button>
          <span className="min-w-0 grow truncate text-xs font-medium text-ink">
            {selectedEntry === null
              ? t("sftp.selectedCountSize", {
                  count: selectedEntries.length,
                  size: formatBytes(selectedEntries.reduce((sum, entry) => sum + (entry.type === "file" ? entry.size : 0), 0)),
                })
              : t("sftp.selected", { name: selectedEntry.name })}
          </span>
          <button type="button" onClick={() => { void upload(); }} disabled={busy || !canUpload}
            title={canUpload ? undefined : t("sftp.local.connectRemote")}
            className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.local.upload")}</button>
        </> : <span className="min-w-0 grow truncate text-xs text-ink-muted">
          {remote?.alias ? t(dragging ? "sftp.dropNow" : "sftp.local.dropHint") : t("sftp.local.connectRemote")}
        </span>}
      </div>
      {listing !== null && pendingPath !== null ? <div role="status" className="absolute inset-0 z-10 flex items-center justify-center rounded-md bg-card/80 p-4 backdrop-blur-[1px]"><span className="flex min-w-0 items-center gap-3 rounded-lg bg-toolbar px-4 py-3 text-sm"><span aria-hidden="true" className="size-4 shrink-0 animate-spin rounded-full border-2 border-accent border-t-transparent motion-reduce:animate-none" /><span className="min-w-0"><span className="block">{t("sftp.local.loading")}</span><span className="block truncate font-mono text-xs text-ink-muted">{pendingPath || t("sftp.homeDirectory")}</span></span></span></div> : null}
      <div data-testid="sftp-file-list" className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain" inert={listing !== null && busy} onKeyDown={list.handleListKeys}>
        {listing === null && busy ? (
          <PanelState tone="loading" title={t("sftp.local.loading")} />
        ) : listingFailed ? (
          <PanelState tone="failed" title={problem} action={<Button onClick={() => void navigate(requestedPath.current)}>{t("sftp.retry")}</Button>} />
        ) : listing === null ? null : displayedEntries.length === 0 ? (
          <PanelState
            tone="empty"
            title={t("sftp.emptyDirectory")}
            action={parentRowVisible ? <Button disabled={busy} onClick={openParent}>{t("sftp.parentDirectory")}</Button> : undefined}
          />
        ) : (
          <SFTPEntryList
            model={list}
            entries={displayedEntries}
            sort={sort}
            onSort={changeSort}
            compact={compactViewport}
            mobileInteraction={mobileInteraction}
            busy={busy}
            parentRowVisible={parentRowVisible}
          />
        )}
      </div>
    </div>
    <ModalShell open={mobileActionsOpen} labelledBy={actionsHeadingId} onDismiss={() => setMobileActionsOpen(false)} placement="sheet" panelClassName="w-full max-w-md rounded-xl p-3">
      <h2 id={actionsHeadingId} className="mb-2 font-semibold">{t("sftp.mobile.actions")}</h2>
      <div className="grid gap-1">
        <button type="button" disabled={busy || history.index < 0 || history.index >= history.paths.length - 1}
          onClick={() => { setMobileActionsOpen(false); void navigateHistory(1); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.forward")}</button>
        <button type="button" disabled={busy || listing === null}
          onClick={() => { setMobileActionsOpen(false); void navigate(""); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.homeDirectory")}</button>
        <button type="button" disabled={busy || listing === null || listing.path === localRoot(listing.path)}
          onClick={() => { setMobileActionsOpen(false); if (listing !== null) void navigate(localRoot(listing.path)); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.rootDirectory")}</button>
        <button type="button" disabled={busy || displayedEntries.length === 0}
          onClick={() => { setMobileActionsOpen(false); list.selectAllDisplayed(); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.selectAll")}</button>
        {(["name", "type", "size", "modified"] as const).map((key) => <button key={key} type="button"
          onClick={() => { setMobileActionsOpen(false); changeSort(key); }} className="rounded px-3 py-3 text-left hover:bg-hover">
          {`${t(`sftp.${key}`)}${t(sort.key === key && sort.direction === "ascending" ? "table.sortDescending" : "table.sortAscending")}`}
        </button>)}
        <button type="button" disabled={listing === null}
          onClick={() => { setMobileActionsOpen(false); void copyPath(); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.copyPath")}</button>
        <button type="button" disabled={listing === null}
          onClick={() => { setMobileActionsOpen(false); editPath(); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.local.editPath")}</button>
      </div>
      <button type="button" onClick={() => setMobileActionsOpen(false)} className="mt-2 w-full rounded border border-line px-3 py-2 text-sm">{t("sftp.cancel")}</button>
    </ModalShell>
  </section>;
}

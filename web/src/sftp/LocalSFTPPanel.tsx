import { useCallback, useEffect, useId, useRef, useState, type DragEvent, type MouseEvent } from "react";
import { failureCode } from "../api/client";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { sftpApi, type LocalListing } from "./api";
import { formatBytes } from "./format";
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

export function LocalSFTPPanel({ aliases, hosts, initialPath, remote, onHostChange, onQueueOpen, onDirectoryChange }: {
  aliases: string[];
  hosts?: HostEntry[];
  initialPath: string;
  remote: { alias: string; path: string } | null;
  onHostChange: (alias: string) => void;
  onQueueOpen: () => void;
  onDirectoryChange: (path: string | null) => void;
}) {
  const t = useTranslate();
  const actionsHeadingId = useId();
  const mobileInteraction = useMediaQuery(mobileViewportQuery);
  const [listing, setListing] = useState<LocalListing | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [dragging, setDragging] = useState(false);
  const [pathEditing, setPathEditing] = useState(false);
  const [pathDraft, setPathDraft] = useState("");
  const [history, setHistory] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const [mobileActionsOpen, setMobileActionsOpen] = useState(false);
  const currentPath = useRef("");
  const loadGeneration = useRef(0);
  const startingPath = useRef(initialPath);
  const reportDirectory = useRef(onDirectoryChange);
  reportDirectory.current = onDirectoryChange;
  const completed = useRef(new Set(sftpTransferManager.getSnapshot()
    .filter((job) => job.direction === "remote" && job.status === "completed").map((job) => job.id)));

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
    setBusy(true);
    try {
      const next = await sftpApi.listLocal(path);
      if (generation !== loadGeneration.current) return;
      currentPath.current = next.path;
      setListing(next);
      setSelected(new Set());
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
      if (generation === loadGeneration.current) setBusy(false);
    }
  }, []);

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

  function select(event: MouseEvent<HTMLButtonElement>, name: string) {
    if (event.ctrlKey || event.metaKey) {
      setSelected((current) => { const next = new Set(current); if (next.has(name)) next.delete(name); else next.add(name); return next; });
    } else setSelected(new Set([name]));
  }
  async function upload() {
    if (listing === null || remote === null || !remote.alias || !remote.path) return;
    const entries = listing.entries.filter((entry) => selected.has(entry.name));
    if (entries.length === 0) return;
    setBusy(true);
    try {
      await sftpTransferManager.addRemoteTransfers(entries.map((entry) => ({
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
  return <section className="flex h-full min-h-0 min-w-0 flex-col gap-1.5 md:gap-1" aria-label={t("sftp.local.heading")}
    onDragOver={(event) => { if (event.dataTransfer.types.includes(remoteEntriesMime)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; setDragging(true); } }}
    onDragLeave={() => setDragging(false)} onDrop={(event) => { void acceptRemoteDrop(event); }}>
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
    {problem !== "" ? <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p> : null}
    {listing !== null ? <>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-md border border-line/60 bg-card">
      <div className={`${mobileInteraction && selected.size === 0 ? "hidden" : "flex"} min-h-10 items-center gap-2 border-b border-line/50 bg-toolbar/45 px-2 py-1 md:min-h-8 md:py-0.5`}>
        <button type="button" onClick={() => { void upload(); }} disabled={busy || selected.size === 0 || !remote?.alias || !remote?.path}
          className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.local.upload")}</button>
        <span className="min-w-0 truncate text-xs text-ink-muted">{remote?.alias ? `${remote.alias}:${remote.path}` : t("sftp.local.connectRemote")}</span>
      </div>
      <div className={`min-h-0 flex-1 overflow-auto ${dragging ? "bg-select-fill" : ""}`}>
        {parentPath(listing.path) !== listing.path ? <button type="button" aria-label={t("sftp.local.parent")}
          onClick={() => { void navigate(parentPath(listing.path)); }} disabled={busy}
          className="flex min-h-10 w-full items-center gap-2 border-b border-line/40 px-3 text-left text-sm hover:bg-hover disabled:text-ink-faint">
          <Icon name="groups" className="size-4 text-ink-muted" /><span aria-hidden="true" className="font-mono">..</span>
        </button> : null}
        {listing.entries.map((entry) => <div key={entry.path}
          className={`flex items-center border-t border-line/40 transition-colors ${selected.has(entry.name) ? "bg-select-fill/75" : "hover:bg-hover/55"}`}>
          <label className="flex size-11 shrink-0 items-center justify-center md:size-8">
            <input type="checkbox" aria-label={t("sftp.selectEntry", { name: entry.name })}
              checked={selected.has(entry.name)} disabled={busy}
              onChange={() => setSelected((current) => {
                const next = new Set(current);
                if (next.has(entry.name)) next.delete(entry.name); else next.add(entry.name);
                return next;
              })} className="size-4 accent-accent" />
          </label>
          <button type="button" aria-label={entry.name} onClick={(event) => select(event, entry.name)}
            onDoubleClick={() => { if (entry.type === "directory") void navigate(entry.path); }} aria-pressed={selected.has(entry.name)}
            onKeyDown={(event) => { if (event.key === "Enter" && entry.type === "directory") { event.preventDefault(); void navigate(entry.path); } }}
            title={entry.type === "directory" ? t("sftp.local.openFolder") : undefined}
            className="flex min-h-12 min-w-0 flex-1 items-center gap-2 px-2 py-2 text-left text-sm hover:bg-hover active:bg-select-fill">
            <Icon name={entry.type === "directory" ? "groups" : "config"} className="size-4 shrink-0 text-ink-muted" />
            <span className="min-w-0 flex-1"><span className="block truncate">{entry.name}</span>
              {entry.type === "file" ? <span className="block text-xs text-ink-muted">{formatBytes(entry.size)}</span> : null}</span>
            {entry.type === "directory" ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
          </button>
        </div>)}
      </div>
      {mobileInteraction ? null : <p className="border-t border-line px-3 py-2 text-xs text-ink-muted">{t("sftp.local.dropHint")}</p>}
      </div>
    </> : null}
    <ModalShell open={mobileActionsOpen} labelledBy={actionsHeadingId} onDismiss={() => setMobileActionsOpen(false)} placement="sheet" panelClassName="w-full max-w-md rounded-xl p-3">
      <h2 id={actionsHeadingId} className="mb-2 font-semibold">{t("sftp.mobile.actions")}</h2>
      <div className="grid gap-1">
        <button type="button" disabled={busy || history.index < 0 || history.index >= history.paths.length - 1}
          onClick={() => { setMobileActionsOpen(false); void navigateHistory(1); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.forward")}</button>
        <button type="button" disabled={busy || listing === null}
          onClick={() => { setMobileActionsOpen(false); void navigate(""); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.homeDirectory")}</button>
        <button type="button" disabled={busy || listing === null || listing.path === localRoot(listing.path)}
          onClick={() => { setMobileActionsOpen(false); if (listing !== null) void navigate(localRoot(listing.path)); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.rootDirectory")}</button>
        <button type="button" disabled={listing === null}
          onClick={() => { setMobileActionsOpen(false); void copyPath(); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.copyPath")}</button>
        <button type="button" disabled={listing === null}
          onClick={() => { setMobileActionsOpen(false); editPath(); }} className="rounded px-3 py-3 text-left hover:bg-hover disabled:text-ink-faint">{t("sftp.local.editPath")}</button>
      </div>
      <button type="button" onClick={() => setMobileActionsOpen(false)} className="mt-2 w-full rounded border border-line px-3 py-2 text-sm">{t("sftp.cancel")}</button>
    </ModalShell>
  </section>;
}

import { useCallback, useEffect, useRef, useState, type DragEvent, type MouseEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { sftpApi, type LocalListing } from "./api";
import { formatBytes } from "./format";
import { remoteEntriesMime, type RemoteDragPayload } from "./transfers";
import { sftpTransferManager } from "./transferManager";

function parentPath(value: string): string {
  if (value === "/" || /^[A-Za-z]:\/$/.test(value)) return value;
  const normalized = value.replace(/\\/g, "/").replace(/\/$/, "");
  const index = normalized.lastIndexOf("/");
  if (index < 0) return normalized;
  return index === 0 ? "/" : normalized.slice(0, index);
}
function joinPath(parent: string, name: string): string {
  return `${parent.replace(/\/$/, "")}/${name}`;
}
function crumbs(value: string): { label: string; path: string }[] {
  const normalized = value.replace(/\\/g, "/");
  if (normalized === "/") return [{ label: "/", path: "/" }];
  const root = normalized.startsWith("/") ? "/" : normalized.slice(0, normalized.indexOf("/") + 1);
  const parts = normalized.slice(root.length).split("/").filter(Boolean);
  return [{ label: root, path: root }, ...parts.map((part, index) => ({ label: part, path: `${root}${parts.slice(0, index + 1).join("/")}` }))];
}

export function LocalSFTPPanel({ remote, onQueueOpen, onDirectoryChange }: {
  remote: { alias: string; path: string } | null;
  onQueueOpen: () => void;
  onDirectoryChange: (path: string | null) => void;
}) {
  const t = useTranslate();
  const [listing, setListing] = useState<LocalListing | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [dragging, setDragging] = useState(false);
  const [pathEditing, setPathEditing] = useState(false);
  const [pathDraft, setPathDraft] = useState("");
  const currentPath = useRef("");
  const completed = useRef(new Set(sftpTransferManager.getSnapshot()
    .filter((job) => job.direction === "remote" && job.status === "completed").map((job) => job.id)));

  const navigate = useCallback(async (path: string) => {
    setBusy(true);
    try {
      const next = await sftpApi.listLocal(path);
      currentPath.current = next.path;
      setListing(next);
      setSelected(new Set());
      onDirectoryChange(next.path);
      setProblem("");
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      setBusy(false);
    }
  }, [onDirectoryChange]);

  useEffect(() => { void navigate(""); }, [navigate]);
  useEffect(() => sftpTransferManager.subscribe(() => {
    for (const job of sftpTransferManager.getSnapshot()) {
      if (job.direction !== "remote" || job.operation !== "get" ||
          job.status !== "completed" || completed.current.has(job.id)) continue;
      completed.current.add(job.id);
      if (currentPath.current !== "") void navigate(currentPath.current);
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
  return <section className="flex min-h-0 min-w-0 flex-1 flex-col rounded-lg border border-line bg-card" aria-label={t("sftp.local.heading")}
    onDragOver={(event) => { if (event.dataTransfer.types.includes(remoteEntriesMime)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; setDragging(true); } }}
    onDragLeave={() => setDragging(false)} onDrop={(event) => { void acceptRemoteDrop(event); }}>
    <div className="flex min-h-12 items-center gap-2 border-b border-line px-3">
      <span className="font-medium">{t("sftp.local.heading")}</span>
      <span className="text-xs text-ink-muted">{t("sftp.local.engine")}</span>
      <span className="min-w-0 flex-1" />
      <button type="button" className="rounded p-2 hover:bg-hover disabled:text-ink-faint" aria-label={t("sftp.local.refresh")}
        disabled={listing === null || busy} onClick={() => { void navigate(listing?.path ?? ""); }}><Icon name="sync" className="size-4" /></button>
    </div>
    {problem !== "" ? <p role="alert" className="px-3 py-2 text-sm text-danger">{problem}</p> : null}
    {listing !== null ? <>
      <nav aria-label={t("sftp.local.path")} className="flex min-h-10 items-center gap-1 border-b border-line px-2">
        <button type="button" aria-label={t("sftp.local.parent")} title={t("sftp.local.parent")}
          onClick={() => { void navigate(parentPath(listing.path)); }} disabled={parentPath(listing.path) === listing.path || busy}
          className="rounded p-2 hover:bg-hover disabled:text-ink-faint"><Icon name="chevronRight" className="size-3 -rotate-90" /></button>
        {pathEditing ? <input autoFocus aria-label={t("sftp.local.pathInput")} value={pathDraft}
          onChange={(event) => setPathDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") setPathEditing(false);
            if (event.key === "Enter") { setPathEditing(false); void navigate(pathDraft.trim()); }
          }} className="min-w-0 flex-1 rounded border border-line bg-surface px-2 py-1 text-sm" />
          : <div className="flex min-w-0 flex-1 items-center overflow-x-auto whitespace-nowrap text-sm">
          {crumbs(listing.path).map((crumb, index) => <span key={crumb.path} className="inline-flex items-center">
            {index > 0 ? <Icon name="chevronRight" className="mx-1 size-3 text-ink-faint" /> : null}
            {crumb.path === listing.path ? <span className="px-1 font-medium" aria-current="location">{crumb.path === listing.home ? "~" : crumb.label}</span>
              : <button type="button" onClick={() => { void navigate(crumb.path); }} disabled={busy}
                  className="rounded px-1 py-1 text-ink-muted hover:bg-hover hover:text-ink">{crumb.path === listing.home ? "~" : crumb.label}</button>}
          </span>)}
        </div>}
        <button type="button" aria-label={t("sftp.local.editPath")} title={t("sftp.local.editPath")}
          onClick={() => { setPathDraft(listing.path); setPathEditing((current) => !current); }}
          className="rounded p-2 hover:bg-hover"><Icon name="edit" className="size-3.5" /></button>
      </nav>
      <div className="flex items-center gap-2 border-b border-line px-3 py-2">
        <button type="button" onClick={() => { void upload(); }} disabled={busy || selected.size === 0 || !remote?.alias || !remote?.path}
          className="rounded bg-accent px-3 py-1.5 text-sm text-white disabled:opacity-40">{t("sftp.local.upload")}</button>
        <span className="min-w-0 truncate text-xs text-ink-muted">{remote?.alias ? `${remote.alias}:${remote.path}` : t("sftp.local.connectRemote")}</span>
      </div>
      <div className={`min-h-0 flex-1 overflow-auto ${dragging ? "bg-select-fill" : ""}`}>
        {listing.entries.map((entry) => <button key={entry.path} type="button" onClick={(event) => select(event, entry.name)}
          onDoubleClick={() => { if (entry.type === "directory") void navigate(entry.path); }} aria-pressed={selected.has(entry.name)}
          onKeyDown={(event) => { if (event.key === "Enter" && entry.type === "directory") { event.preventDefault(); void navigate(entry.path); } }}
          title={entry.type === "directory" ? t("sftp.local.openFolder") : undefined}
          className={`flex min-h-10 w-full items-center gap-3 px-3 text-left text-sm hover:bg-hover ${selected.has(entry.name) ? "bg-select-fill" : ""}`}>
          <Icon name={entry.type === "directory" ? "groups" : "config"} className="size-4 shrink-0 text-ink-muted" />
          <span className="min-w-0 flex-1 truncate">{entry.name}</span>
          <span className="shrink-0 text-xs text-ink-muted">{entry.type === "directory" ? "" : formatBytes(entry.size)}</span>
          {entry.type === "directory" ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
        </button>)}
      </div>
      <p className="border-t border-line px-3 py-2 text-xs text-ink-muted">{t("sftp.local.dropHint")}</p>
    </> : null}
  </section>;
}

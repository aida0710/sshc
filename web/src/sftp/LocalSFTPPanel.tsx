import { useCallback, useEffect, useRef, useState, type DragEvent, type MouseEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { sftpApi } from "./api";
import { formatBytes } from "./format";
import { directoryPaths, remoteEntriesMime, safeRelativePath, type LocalTransferFile, type RemoteDragPayload } from "./transfers";
import { sftpTransferManager } from "./transferManager";

type LocalEntry = { handle: FileSystemHandle; size: number | null };
type WritablePicker = Window & {
  showDirectoryPicker?: (options: { id: string; mode: "readwrite" }) => Promise<FileSystemDirectoryHandle>;
};

function childHandles(directory: FileSystemDirectoryHandle): AsyncIterable<FileSystemHandle> {
  return (directory as FileSystemDirectoryHandle & { values(): AsyncIterable<FileSystemHandle> }).values();
}

async function collectLocal(handle: FileSystemHandle, relativePath: string, files: LocalTransferFile[], folders: string[]): Promise<void> {
  const safe = safeRelativePath(relativePath);
  if (safe === null || files.length + folders.length >= 20000) throw new Error("sftp_local_selection_limit");
  if (handle.kind === "file") {
    files.push({ file: await (handle as FileSystemFileHandle).getFile(), relativePath: safe });
    return;
  }
  folders.push(safe);
  for await (const child of childHandles(handle as FileSystemDirectoryHandle)) {
    await collectLocal(child, `${safe}/${child.name}`, files, folders);
  }
}

export function LocalSFTPPanel({ remote, onQueueOpen, onDirectoryChange }: {
  remote: { alias: string; path: string } | null;
  onQueueOpen: () => void;
  onDirectoryChange: (directory: FileSystemDirectoryHandle | null) => void;
}) {
  const t = useTranslate();
  const [stack, setStack] = useState<FileSystemDirectoryHandle[]>([]);
  const [entries, setEntries] = useState<LocalEntry[]>([]);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [dragging, setDragging] = useState(false);
  const [pendingDownloads, setPendingDownloads] = useState(sftpTransferManager.getUnattachedLocalDownloadCount);
  const completed = useRef(new Set(sftpTransferManager.getSnapshot()
    .filter((job) => job.direction === "download" && job.status === "completed").map((job) => job.id)));
  const directory = stack.at(-1);
  const supported = typeof (window as WritablePicker).showDirectoryPicker === "function";

  const refresh = useCallback(async (current: FileSystemDirectoryHandle) => {
    const listed: LocalEntry[] = [];
    for await (const handle of childHandles(current)) {
      const size = handle.kind === "file" ? (await (handle as FileSystemFileHandle).getFile()).size : null;
      listed.push({ handle, size });
    }
    listed.sort((left, right) => left.handle.kind === right.handle.kind
      ? left.handle.name.localeCompare(right.handle.name) : left.handle.kind === "directory" ? -1 : 1);
    setEntries(listed);
    setSelected(new Set());
  }, []);

  useEffect(() => {
    return sftpTransferManager.subscribe(() => setPendingDownloads(sftpTransferManager.getUnattachedLocalDownloadCount()));
  }, []);

  useEffect(() => {
    if (directory === undefined) return;
    return sftpTransferManager.subscribe(() => {
      for (const job of sftpTransferManager.getSnapshot()) {
        if (job.direction !== "download" || job.status !== "completed" || completed.current.has(job.id)) continue;
        completed.current.add(job.id);
        void refresh(directory).catch(() => undefined);
      }
    });
  }, [directory, refresh]);

  async function chooseFolder() {
    const picker = (window as WritablePicker).showDirectoryPicker;
    if (picker === undefined) return;
    setProblem("");
    try {
      const root = await picker.call(window, { id: "sshc-sftp-local", mode: "readwrite" });
      await refresh(root);
      setStack([root]);
      onDirectoryChange(root);
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setProblem(error instanceof Error ? error.message : "sftp_local_access_failed");
    }
  }

  async function enter(entry: LocalEntry) {
    if (entry.handle.kind !== "directory") return;
    const next = entry.handle as FileSystemDirectoryHandle;
    setBusy(true);
    try {
      await refresh(next);
      setStack((current) => [...current, next]);
      onDirectoryChange(next);
      setProblem("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "sftp_local_access_failed");
    } finally {
      setBusy(false);
    }
  }

  async function goUp() {
    if (stack.length < 2) return;
    const parent = stack.at(-2)!;
    setBusy(true);
    try {
      await refresh(parent);
      setStack((current) => current.slice(0, -1));
      onDirectoryChange(parent);
      setProblem("");
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "sftp_local_access_failed");
    } finally {
      setBusy(false);
    }
  }

  function select(event: MouseEvent<HTMLButtonElement>, name: string) {
    if (event.ctrlKey || event.metaKey) {
      setSelected((current) => {
        const next = new Set(current);
        if (next.has(name)) next.delete(name);
        else next.add(name);
        return next;
      });
      return;
    }
    setSelected(new Set([name]));
  }

  async function upload() {
    if (remote === null || remote.alias === "" || remote.path === "" || selected.size === 0) return;
    setBusy(true);
    setProblem("");
    let admission: ReturnType<typeof sftpTransferManager.reserveUploads> | undefined;
    try {
      const files: LocalTransferFile[] = [];
      const folders: string[] = [];
      for (const entry of entries) {
        if (selected.has(entry.handle.name)) await collectLocal(entry.handle, entry.handle.name, files, folders);
      }
      const selections = files.map(({ file, relativePath }) => ({
        alias: remote.alias, remotePath: `${remote.path.replace(/\/$/, "")}/${relativePath}`, localName: relativePath, file,
      }));
      admission = sftpTransferManager.reserveUploads(selections);
      const allFolders = [...new Set([...folders, ...directoryPaths(files)])]
        .sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
      for (const folder of allFolders) {
        try {
          await sftpApi.mkdir(remote.alias, `${remote.path.replace(/\/$/, "")}/${folder}`);
        } catch (error) {
          if (failureCode(error) !== "sftp_exists") throw error;
        }
      }
      if (selections.length > 0) {
        await sftpTransferManager.addUploads(selections, {
          name: [...selected][0] ?? t("sftp.local.heading"),
          kind: folders.length > 0 || selections.length > 1 ? "folder" : "file",
        }, admission);
        admission = undefined;
        onQueueOpen();
      }
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      admission?.release();
      setBusy(false);
    }
  }

  async function acceptRemoteDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault();
    setDragging(false);
    if (directory === undefined || busy) return;
    try {
      const payload = JSON.parse(event.dataTransfer.getData(remoteEntriesMime)) as RemoteDragPayload;
      if (typeof payload.alias !== "string" || payload.alias === "" || !Array.isArray(payload.entries) ||
          payload.entries.some((entry) => typeof entry.path !== "string" ||
            (entry.type !== "file" && entry.type !== "directory") || typeof entry.size !== "number")) {
        throw new Error("sftp_local_drop_invalid");
      }
      for (const entry of payload.entries) {
        await sftpTransferManager.addDownload(payload.alias, entry.path,
          entry.type === "directory" ? "folder" : "file", entry.type === "file" ? entry.size : -1,
          { directory });
      }
      onQueueOpen();
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_local_drop_invalid"));
    }
  }

  async function resumeLocalDownloads() {
    if (directory === undefined) return;
    setBusy(true);
    setProblem("");
    try {
      await sftpTransferManager.attachLocalDownloadDirectory(directory);
      setPendingDownloads(sftpTransferManager.getUnattachedLocalDownloadCount());
      onQueueOpen();
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col rounded-lg border border-line bg-card" aria-label={t("sftp.local.heading")}
      onDragOver={(event) => { if (event.dataTransfer.types.includes(remoteEntriesMime)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; setDragging(true); } }}
      onDragLeave={() => setDragging(false)} onDrop={(event) => { void acceptRemoteDrop(event); }}>
      <div className="flex min-h-12 items-center gap-2 border-b border-line px-3">
        <span className="font-medium">{t("sftp.local.heading")}</span>
        <span className="min-w-0 flex-1 truncate text-xs text-ink-muted" title={stack.map((handle) => handle.name).join("/")}>{stack.map((handle) => handle.name).join("/")}</span>
        <button type="button" className="rounded px-2 py-1 text-sm hover:bg-hover" onClick={() => { void chooseFolder(); }} disabled={!supported}>{t("sftp.local.choose")}</button>
        <button type="button" className="rounded p-2 hover:bg-hover disabled:text-ink-faint" aria-label={t("sftp.local.refresh")}
          disabled={directory === undefined || busy} onClick={() => { if (directory) void refresh(directory).catch((error: unknown) => setProblem(String(error))); }}>
          <Icon name="sync" className="size-4" />
        </button>
      </div>
      {problem !== "" ? <p role="alert" className="px-3 py-2 text-sm text-danger">{problem}</p> : null}
      {!supported ? <p className="p-4 text-sm text-ink-muted">{t("sftp.local.unsupported")}</p> : null}
      {supported && directory === undefined ? <p className="p-4 text-sm text-ink-muted">{t("sftp.local.chooseHint")}</p> : null}
      {directory !== undefined ? <>
        <div className="flex items-center gap-2 border-b border-line px-3 py-2">
          <button type="button" onClick={() => { void goUp(); }} disabled={stack.length < 2 || busy} className="rounded px-2 py-1 text-sm hover:bg-hover disabled:text-ink-faint">..</button>
          <button type="button" onClick={() => { void upload(); }} disabled={busy || selected.size === 0 || remote?.alias === "" || !remote?.path}
            className="rounded bg-accent px-3 py-1.5 text-sm text-white disabled:opacity-40">{t("sftp.local.upload")}</button>
          <span className="min-w-0 truncate text-xs text-ink-muted">{remote?.alias ? `${remote.alias}:${remote.path}` : t("sftp.local.connectRemote")}</span>
        </div>
        {pendingDownloads > 0 ? <button type="button" onClick={() => { void resumeLocalDownloads(); }} disabled={busy}
          className="border-b border-line px-3 py-2 text-left text-sm text-accent hover:bg-hover disabled:opacity-40">
          {t("sftp.local.resumeDownloads", { count: pendingDownloads })}
        </button> : null}
        <div className={`min-h-0 flex-1 overflow-auto ${dragging ? "bg-select-fill" : ""}`}>
          {entries.map((entry) => (
            <button key={entry.handle.name} type="button" onClick={(event) => select(event, entry.handle.name)}
              onDoubleClick={() => { void enter(entry); }} aria-pressed={selected.has(entry.handle.name)}
              className={`flex min-h-10 w-full items-center gap-3 px-3 text-left text-sm hover:bg-hover ${selected.has(entry.handle.name) ? "bg-select-fill" : ""}`}>
              <Icon name={entry.handle.kind === "directory" ? "groups" : "config"} className="size-4 shrink-0 text-ink-muted" />
              <span className="min-w-0 flex-1 truncate">{entry.handle.name}</span>
              <span className="shrink-0 text-xs text-ink-muted">{entry.size === null ? "" : formatBytes(entry.size)}</span>
            </button>
          ))}
        </div>
        <p className="border-t border-line px-3 py-2 text-xs text-ink-muted">{t("sftp.local.dropHint")}</p>
      </> : null}
    </section>
  );
}

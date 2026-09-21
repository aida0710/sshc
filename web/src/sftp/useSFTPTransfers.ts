import { useEffect, useRef, useState, useSyncExternalStore, type ChangeEvent, type DragEvent as ReactDragEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sftpApi, type RemoteEntry } from "./api";
import { localHostAlias } from "./localHost";
import { localJoin, localParentOf, remoteParentOf, sourceFor } from "./sftpSource";
import { entryKind, movable } from "./entryKind";
import { sftpTransferManager } from "./transferManager";
import { directoryPaths, remoteEntriesMime, safeRelativePath, type LocalTransferFile, type RemoteDragPayload } from "./transfers";
import { payloadFor, registerDrag, releaseDrag } from "./dragRegistry";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

type DroppedEntry = {
  isFile: boolean;
  isDirectory: boolean;
  name: string;
  file?: (success: (file: File) => void, failure?: (error: DOMException) => void) => void;
  createReader?: () => { readEntries: (success: (entries: DroppedEntry[]) => void, failure?: (error: DOMException) => void) => void };
};

async function droppedFiles(transfer: DataTransfer): Promise<{ files: LocalTransferFile[]; directories: string[] }> {
  const collected: LocalTransferFile[] = [];
  const directories = new Set<string>();
  const visit = async (entry: DroppedEntry, prefix: string): Promise<void> => {
    const relativePath = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
    if (entry.isFile && entry.file !== undefined) {
      const file = await new Promise<File>((resolve, reject) => entry.file?.(resolve, reject));
      const safe = safeRelativePath(relativePath);
      if (safe !== null) collected.push({ file, relativePath: safe });
      return;
    }
    if (!entry.isDirectory || entry.createReader === undefined) return;
    const safeDirectory = safeRelativePath(relativePath);
    if (safeDirectory !== null) directories.add(safeDirectory);
    const reader = entry.createReader();
    while (true) {
      const children = await new Promise<DroppedEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
      if (children.length === 0) break;
      for (const child of children) await visit(child, relativePath);
    }
  };
  const items = [...(transfer.items ?? [])];
  const entries = items.map((item) => (item as DataTransferItem & { webkitGetAsEntry?: () => DroppedEntry | null }).webkitGetAsEntry?.() ?? null);
  if (entries.some((entry) => entry !== null)) {
    for (const entry of entries) if (entry !== null) await visit(entry, "");
    return { files: collected, directories: [...directories] };
  }
  return { files: [...transfer.files].flatMap((file) => {
    const safe = safeRelativePath(file.name);
    return safe === null ? [] : [{ file, relativePath: safe }];
  }), directories: [] };
}

// A dragged row's name becomes the last segment of the target path, so it
// must be exactly one safe segment.
function validEntryName(name: string): boolean {
  return safeRelativePath(name) === name && !name.includes("/");
}

function transferable<T extends { type: RemoteEntry["type"] }>(entries: T[]): T[] {
  return entries.filter(movable);
}

export type SFTPCounterpart = { alias: string; path: string };

// Every way bytes move into or out of the directory a pane shows, and the
// one place that decides which engine operation a pair of sources needs:
// browser files land as uploads, rows dragged between two hosts copy or
// move, and anything between the engine's disk and a host is a put or a get.
export function useSFTPTransfers({
  browser,
  selectedEntries,
  busy,
  counterpart = null,
  onQueueOpen,
}: {
  browser: SFTPBrowserModel;
  selectedEntries: RemoteEntry[];
  busy: boolean;
  // The other visible pane, when there is one. A download from a host next
  // to the engine's disk goes straight there instead of to the browser.
  counterpart?: SFTPCounterpart | null;
  onQueueOpen?: (() => void) | undefined;
}) {
  const t = useTranslate();
  const { alias, path, connected, source, setProblem } = browser;
  const [dragging, setDragging] = useState(false);
  const [remoteDrop, setRemoteDrop] = useState<RemoteDragPayload | null>(null);
  const [queuing, setQueuing] = useState(false);
  const [openQueueRequest, setOpenQueueRequest] = useState(0);
  const uploadInput = useRef<HTMLInputElement>(null);
  const folderUploadInput = useRef<HTMLInputElement>(null);
  const transferJobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const refreshedJobs = useRef(new Set<string>());
  const local = source?.local === true;
  const counterpartLocal = counterpart !== null && counterpart.alias === localHostAlias;
  const counterpartRemote = counterpart !== null && !counterpartLocal && counterpart.alias !== "";

  function report(error: unknown, fallback = "sftp_failed") {
    setProblem(failureCode(error) || (error instanceof Error ? error.message : fallback));
  }

  function openQueue() {
    setOpenQueueRequest((current) => current + 1);
    onQueueOpen?.();
  }

  // A completed job that wrote into the directory on screen changes what the
  // rows should say. Deletions are the pane's business: their refresh has to
  // respect a search in progress.
  useEffect(() => {
    if (!connected || path === "") return;
    const arrived = transferJobs.filter((job) => job.status === "completed" && !refreshedJobs.current.has(job.id) && (local
      ? job.operation === "get" && localParentOf(job.remotePath) === path
      : (job.direction === "upload" || job.operation === "put") && job.alias === alias && remoteParentOf(job.remotePath) === path));
    if (arrived.length === 0) return;
    for (const job of arrived) refreshedJobs.current.add(job.id);
    void browser.refresh();
    // `browser` is a fresh object each render; only a change in the jobs or
    // in the directory on screen should trigger a refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transferJobs, alias, path, connected, local]);

  async function uploadFiles(files: LocalTransferFile[], droppedDirectories: string[] = []) {
    if (alias === "" || local || (files.length === 0 && droppedDirectories.length === 0) || busy) return;
    const safeFiles = files.flatMap((item) => {
      const relativePath = safeRelativePath(item.relativePath);
      return relativePath === null ? [] : [{ file: item.file, relativePath }];
    });
    const safeDirectories = droppedDirectories.flatMap((directory) => {
      const safe = safeRelativePath(directory);
      return safe === null ? [] : [safe];
    });
    if (safeFiles.length === 0 && safeDirectories.length === 0) return;
    const join = (name: string) => browser.source?.join(path, name) ?? `${path}/${name}`;
    setQueuing(true);
    setProblem("");
    const selections = safeFiles.map((item) => ({
      alias, remotePath: join(item.relativePath), localName: item.relativePath, file: item.file,
    }));
    let admission: ReturnType<typeof sftpTransferManager.reserveUploads> | undefined;
    try {
      admission = sftpTransferManager.reserveUploads(selections);
      const directories = [...new Set([...directoryPaths(safeFiles), ...safeDirectories])]
        .sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
      for (const directory of directories) {
        try {
          await sftpApi.mkdir(alias, join(directory));
        } catch (error) {
          if (failureCode(error) !== "sftp_exists") throw error;
        }
      }
      const folderName = [...safeDirectories, ...safeFiles.map((item) => item.relativePath)]
        .map((value) => value.split("/")[0] ?? value).find((value) => value !== "") ?? t("sftp.manager.folder");
      const folderBatch = safeDirectories.length > 0 || safeFiles.length > 1 || safeFiles.some((item) => item.relativePath.includes("/"));
      await sftpTransferManager.addUploads(selections, {
        name: folderBatch ? folderName : safeFiles[0]?.relativePath ?? folderName,
        kind: folderBatch ? "folder" : "file",
      }, admission);
      admission = undefined;
    } catch (error) {
      report(error);
    } finally {
      admission?.release();
      setQueuing(false);
    }
  }

  function uploadFromInput(event: ChangeEvent<HTMLInputElement>, relative: (file: File) => string) {
    const files = Array.from(event.target.files ?? []).flatMap((file) => {
      const relativePath = safeRelativePath(relative(file));
      return relativePath === null ? [] : [{ file, relativePath }];
    });
    event.target.value = "";
    void uploadFiles(files);
  }

  // Engine-side transfers between this source and another. `put` sends the
  // engine's files to a host, `get` brings a host's files to the engine.
  async function queueEngineTransfers(operation: "put" | "get", jobs: Parameters<typeof sftpTransferManager.addRemoteTransfers>[0]) {
    setQueuing(true);
    try {
      await sftpTransferManager.addRemoteTransfers(jobs, operation);
      openQueue();
      setProblem("");
    } catch (error) {
      report(error);
    } finally {
      setQueuing(false);
    }
  }

  // Rows dropped from another pane. Between two hosts the user still chooses
  // copy or move; to or from the engine's disk there is only one meaning, and
  // within one host a drop moves, as it does in a desktop file manager. Rows
  // dropped back into the directory they came from are left alone.
  async function acceptPayload(payload: RemoteDragPayload) {
    if (source === null || path === "") return;
    if (local) {
      if (payload.alias === localHostAlias) return;
      await queueEngineTransfers("get", transferable(payload.entries).map((entry) => ({
        sourceAlias: payload.alias, sourcePath: entry.path,
        targetAlias: payload.alias, targetPath: localJoin(path, entry.name),
        name: entry.name, kind: entry.type === "directory" ? "folder" : "file", totalBytes: entry.size,
      })));
      return;
    }
    if (payload.alias === localHostAlias) {
      await queueEngineTransfers("put", payload.entries.map((entry) => ({
        sourceAlias: alias, sourcePath: entry.path,
        targetAlias: alias, targetPath: source.join(path, entry.name),
        name: entry.name, kind: entry.type === "directory" ? "folder" : "file", totalBytes: entry.size,
      })));
      return;
    }
    if (payload.alias === alias) {
      const fromElsewhere = payload.entries.filter((entry) => remoteParentOf(entry.path) !== path);
      if (fromElsewhere.length > 0) await queueRemoteTransfers({ alias: payload.alias, entries: fromElsewhere }, "move");
      return;
    }
    setRemoteDrop(payload);
  }

  async function acceptDrop(event: ReactDragEvent<HTMLElement>) {
    event.preventDefault();
    setDragging(false);
    if (busy || alias === "" || !connected) return;
    const token = typeof event.dataTransfer.getData === "function"
      ? event.dataTransfer.getData(remoteEntriesMime)
      : "";
    if (token !== "") {
      const payload = payloadFor(token);
      if (payload === null || !payload.entries.every((entry) => validEntryName(entry.name))) {
        setProblem(t("sftp.remoteDropInvalid"));
        return;
      }
      await acceptPayload(payload);
      return;
    }
    if (local) return;
    const selection = await droppedFiles(event.dataTransfer);
    await uploadFiles(selection.files, selection.directories);
  }

  function dragEnter(event: ReactDragEvent<HTMLElement>) {
    event.preventDefault();
    if (busy || !connected) return;
    // The engine's disk takes rows from a host, never files from the browser.
    if (local && !event.dataTransfer.types.includes(remoteEntriesMime)) return;
    setDragging(true);
  }

  function dragOver(event: ReactDragEvent<HTMLElement>) {
    if (local && !event.dataTransfer.types.includes(remoteEntriesMime)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
  }

  function dragLeave(event: ReactDragEvent<HTMLElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false);
  }

  function beginDrag(event: ReactDragEvent<HTMLElement>, entry: RemoteEntry, selectedPaths: Set<string>) {
    if (entry.type !== "file" && entry.type !== "directory") {
      event.preventDefault();
      return;
    }
    const selected = selectedPaths.has(entry.path) ? selectedEntries : [entry];
    const payload: RemoteDragPayload = {
      alias,
      entries: transferable(selected).map((candidate) => ({ name: candidate.name, path: candidate.path, type: candidate.type, size: candidate.size })),
    };
    event.dataTransfer.effectAllowed = "copyMove";
    const token = registerDrag(payload);
    event.dataTransfer.setData(remoteEntriesMime, token);
    event.dataTransfer.setData("text/plain", payload.entries.map((candidate) => `${alias}:${candidate.path}`).join("\n"));
    event.currentTarget.addEventListener("dragend", () => releaseDrag(token), { once: true });
  }

  async function acceptRemoteDrop(operation: "copy" | "move") {
    const payload = remoteDrop;
    setRemoteDrop(null);
    if (payload !== null) await queueRemoteTransfers(payload, operation);
  }

  // Copies or moves rows from another host, or from another directory of
  // this host, into the directory shown here.
  async function queueRemoteTransfers(payload: RemoteDragPayload, operation: "copy" | "move") {
    if (source === null || alias === "" || path === "") return;
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(payload.entries.map((entry) => ({
        sourceAlias: payload.alias,
        sourcePath: entry.path,
        targetAlias: alias,
        targetPath: source.join(path, entry.name),
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name,
        totalBytes: entry.type === "file" ? entry.size : -1,
      })), operation);
    } catch (error) {
      report(error);
    }
  }

  // Whether "send it over" can take this entry: the browser receives what a
  // symlink points to, while the engine's own transfers take only files and
  // directories named directly.
  function sendable(entry: RemoteEntry): boolean {
    return local || counterpartLocal ? movable(entry) : entryKind(entry) !== "other";
  }

  // From a host: to the browser, or straight to the engine's disk when the
  // other pane shows it. From the engine's disk: to the host in the other pane.
  async function transferOut(targets: RemoteEntry[], targetAlias = alias) {
    const entries = targets.filter(sendable);
    if (busy || entries.length === 0) return;
    setProblem("");
    // Decide from the alias the rows belong to, not from this render's pane:
    // a terminal link can name a host while the pane still shows the engine's
    // disk, and the closure that runs then predates the host switch.
    if (sourceFor(targetAlias)?.local === true) {
      if (!counterpartRemote || counterpart === null) return;
      await queueEngineTransfers("put", entries.map((entry) => ({
        sourceAlias: counterpart.alias, sourcePath: entry.path,
        targetAlias: counterpart.alias, targetPath: `${counterpart.path.replace(/\/$/, "")}/${entry.name}`,
        name: entry.name, kind: entry.type === "directory" ? "folder" : "file", totalBytes: entry.size,
      })));
      return;
    }
    const results = await Promise.allSettled(entries.map((entry) => {
      const kind = entryKind(entry) === "directory" ? "folder" : "file";
      const size = entryKind(entry) === "file" ? entry.size : -1;
      return counterpartLocal && counterpart !== null
        ? sftpTransferManager.addRemoteTransfers([{ sourceAlias: targetAlias, sourcePath: entry.path, targetAlias,
            targetPath: localJoin(counterpart.path, entry.name), name: entry.name, kind, totalBytes: size }], "get")
        : sftpTransferManager.addDownload(targetAlias, entry.path, kind, size);
    }));
    const failed = results.find((result) => result.status === "rejected");
    if (failed?.status === "rejected") report(failed.reason);
  }

  return {
    dragging,
    sendable,
    dragEnter,
    dragOver,
    dragLeave,
    acceptDrop,
    beginDrag,
    remoteDrop,
    acceptRemoteDrop,
    dismissRemoteDrop: () => setRemoteDrop(null),
    uploadInput,
    folderUploadInput,
    uploadFromInput,
    chooseFiles: () => uploadInput.current?.click(),
    chooseFolder: () => folderUploadInput.current?.click(),
    transferOut,
    // Whether "send to the other pane" has somewhere to go.
    canTransferOut: local ? counterpartRemote : true,
    queuing,
    openQueueRequest,
    openQueue,
  };
}


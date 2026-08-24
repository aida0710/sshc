import { Suspense, lazy, useEffect, useRef, useState, type DragEvent as ReactDragEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import {
  compareText,
  nextSort,
  ordered,
  SortableTableHeader,
  type SortDirection,
} from "../ui/tableSort";
import { sftpApi, type RemoteEntry, type RemoteTextFile } from "./api";
import { directoryPaths, safeRelativePath, symbolicModeToOctal, type LocalTransferFile } from "./transfers";

const MonacoEditor = lazy(() =>
  import("./MonacoEditor").then(({ MonacoEditor }) => ({ default: MonacoEditor })),
);

function parentOf(remotePath: string): string {
  if (remotePath === "/") return "/";
  const pieces = remotePath.split("/").filter(Boolean);
  pieces.pop();
  return `/${pieces.join("/")}` || "/";
}

function join(parent: string, name: string): string {
  return `${parent === "/" ? "" : parent}/${name}`;
}

type UploadItem = { id: string; name: string; size: number; status: "pending" | "uploading" | "done" | "failed" | "skipped" | "cancelled"; problem?: string };
type SFTPSort = "name" | "type" | "size" | "modified" | "mode";
type TransferProgress = { kind: "upload" | "download"; processed: number; total: number; bytes: number; totalBytes: number | null };
type OverwriteDecision = { path: string; decide: (overwrite: boolean) => void };

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

export function SFTPPanel({ aliases }: { aliases: string[] }) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [path, setPath] = useState("/");
  const [pathDraft, setPathDraft] = useState("/");
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [opened, setOpened] = useState<RemoteTextFile | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [deleting, setDeleting] = useState<RemoteEntry | null>(null);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [progress, setProgress] = useState<TransferProgress | null>(null);
  const [overwriteDecision, setOverwriteDecision] = useState<OverwriteDecision | null>(null);
  const [dragging, setDragging] = useState(false);
  const [sort, setSort] = useState<{ key: SFTPSort; direction: SortDirection }>({
    key: "name",
    direction: "ascending",
  });
  const upload = useRef<HTMLInputElement>(null);
  const folderUpload = useRef<HTMLInputElement>(null);
  const transferAbort = useRef<AbortController | null>(null);
  const cancelled = useRef(false);
  const dirty = opened !== null && contents !== opened.contents;
  const displayedEntries = ordered(
    entries,
    (left, right) => {
      switch (sort.key) {
        case "type": return compareText(left.type, right.type);
        case "size": return left.size - right.size;
        case "modified": return Date.parse(left.modifiedAt) - Date.parse(right.modifiedAt);
        case "mode": return compareText(left.mode, right.mode);
        case "name": return compareText(left.name, right.name);
      }
    },
    sort.direction,
  );

  function changeSort(key: SFTPSort) {
    setSort((current) => nextSort(current.key, current.direction, key));
  }

  async function load(nextPath = path, nextAlias = alias, preserveEditor = false) {
    if (nextAlias === "") return;
    setBusy(true);
    setProblem("");
    try {
      const listing = await sftpApi.list(nextAlias, nextPath);
      setPath(listing.path);
      setPathDraft(listing.path);
      setEntries(listing.entries);
      if (!preserveEditor) {
        setOpened(null);
        setContents("");
      }
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (alias !== "") void load("/", alias);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [alias]);

  async function openText(entry: RemoteEntry) {
    if (dirty) {
      setProblem(t("sftp.unsavedBlocked"));
      return;
    }
    setBusy(true);
    setProblem("");
    try {
      const file = await sftpApi.readText(alias, entry.path);
      setOpened(file);
      setContents(file.contents);
    } catch (error) {
      const code = failureCode(error);
      if (code === "sftp_not_utf8" || code === "sftp_text_too_large") {
        setProblem(t(code === "sftp_not_utf8" ? "sftp.binaryHint" : "sftp.tooLargeHint"));
      } else {
        setProblem(code || (error instanceof Error ? error.message : "sftp_failed"));
      }
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (opened === null) return;
    setBusy(true);
    setProblem("");
    try {
      const saved = await sftpApi.saveText(alias, opened.entry.path, contents, opened.revision);
      setOpened(saved);
      setContents(saved.contents);
      await load(parentOf(saved.entry.path));
      setOpened(saved);
      setContents(saved.contents);
    } catch (error) {
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
    } finally {
      setBusy(false);
    }
  }

  async function makeDirectory() {
    const name = window.prompt(t("sftp.mkdirPrompt"))?.trim() ?? "";
    if (name === "" || name.includes("/")) return;
    setBusy(true);
    try {
      await sftpApi.mkdir(alias, join(path, name));
      await load();
    } catch (error) {
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function rename(entry: RemoteEntry) {
    const name = window.prompt(t("sftp.renamePrompt"), entry.name)?.trim() ?? "";
    if (name === "" || name === entry.name || name.includes("/")) return;
    setBusy(true);
    try {
      await sftpApi.rename(alias, entry.path, join(path, name));
      await load();
    } catch (error) {
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function remove() {
    if (deleting === null) return;
    setBusy(true);
    try {
      await sftpApi.remove(alias, deleting.path);
      setDeleting(null);
      await load();
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "delete_failed"));
      setDeleting(null);
      setBusy(false);
    }
  }

  async function uploadFiles(files: LocalTransferFile[], droppedDirectories: string[] = []) {
    if (alias === "" || (files.length === 0 && droppedDirectories.length === 0) || busy) return;
    const safeFiles = files.flatMap((item) => {
      const relativePath = safeRelativePath(item.relativePath);
      return relativePath === null ? [] : [{ file: item.file, relativePath }];
    });
    const safeDirectories = droppedDirectories.flatMap((directory) => {
      const safe = safeRelativePath(directory);
      return safe === null ? [] : [safe];
    });
    if (safeFiles.length === 0 && safeDirectories.length === 0) return;
    const batch = safeFiles.map((item) => ({ id: crypto.randomUUID(), name: item.relativePath, size: item.file.size, status: "pending" as const }));
    setUploads(batch);
    setProgress(null);
    setBusy(true);
    cancelled.current = false;
    const controller = new AbortController();
    transferAbort.current = controller;
    const totalBytes = safeFiles.reduce((sum, item) => sum + item.file.size, 0);
    let processed = 0;
    let processedBytes = 0;
    if (safeFiles.length > 0) setProgress({ kind: "upload", processed: 0, total: safeFiles.length, bytes: 0, totalBytes });
    const directories = [...new Set([...directoryPaths(safeFiles), ...safeDirectories])]
      .sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
    for (const directory of directories) {
      if (cancelled.current) break;
      try {
        await sftpApi.mkdir(alias, join(path, directory));
      } catch (error) {
        if (failureCode(error) !== "sftp_exists") {
          setProblem(failureCode(error) || "sftp_failed");
          cancelled.current = true;
        }
      }
    }
    for (let index = 0; index < safeFiles.length; index++) {
      const source = safeFiles[index];
      const item = batch[index];
      if (source === undefined || item === undefined) continue;
      if (cancelled.current) {
        setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "cancelled" } : candidate));
        continue;
      }
      setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "uploading" } : candidate));
      try {
        const remotePath = join(path, source.relativePath);
        try {
          await sftpApi.upload(alias, remotePath, source.file, false, controller.signal);
        } catch (error) {
          if (failureCode(error) !== "sftp_exists") throw error;
          const overwrite = await new Promise<boolean>((decide) => setOverwriteDecision({ path: remotePath, decide }));
          if (cancelled.current) throw new DOMException("cancelled", "AbortError");
          if (!overwrite) {
            setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "skipped" } : candidate));
            continue;
          }
          await sftpApi.upload(alias, remotePath, source.file, true, controller.signal);
        }
        setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "done" } : candidate));
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "cancelled" } : candidate));
          cancelled.current = true;
          continue;
        }
        const uploadProblem = failureCode(error) || (error instanceof Error ? error.message : "upload_failed");
        setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "failed", problem: uploadProblem } : candidate));
      } finally {
        processed += 1;
        processedBytes += source.file.size;
        setProgress({ kind: "upload", processed, total: safeFiles.length, bytes: processedBytes, totalBytes });
      }
    }
    transferAbort.current = null;
    setOverwriteDecision(null);
    await load(path, alias, true);
  }

  async function acceptDrop(event: ReactDragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    if (busy || alias === "") return;
    const selection = await droppedFiles(event.dataTransfer);
    await uploadFiles(selection.files, selection.directories);
  }

  function cancelTransfer() {
    cancelled.current = true;
    transferAbort.current?.abort();
    overwriteDecision?.decide(false);
    setOverwriteDecision(null);
  }

  async function download(entry: RemoteEntry) {
    if (busy) return;
    setBusy(true);
    setProblem("");
    cancelled.current = false;
    const controller = new AbortController();
    transferAbort.current = controller;
    setProgress({ kind: "download", processed: 0, total: 1, bytes: 0, totalBytes: entry.type === "file" ? entry.size : null });
    try {
      await sftpApi.download(alias, entry.path, entry.type === "directory", {
        signal: controller.signal,
        onProgress: (bytes, total) => setProgress({ kind: "download", processed: 0, total: 1, bytes, totalBytes: total }),
      });
      setProgress((current) => current === null ? current : { ...current, processed: 1 });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) setProblem(error instanceof Error ? error.message : "download_failed");
    } finally {
      transferAbort.current = null;
      setBusy(false);
    }
  }

  async function chmod(entry: RemoteEntry) {
    if (entry.type === "symlink" || entry.type === "other") return;
    const mode = window.prompt(t("sftp.chmodPrompt"), symbolicModeToOctal(entry.mode))?.trim() ?? "";
    if (!/^0?[0-7]{3}$/.test(mode)) return;
    setBusy(true);
    try {
      await sftpApi.chmod(alias, entry.path, mode, entry.revision);
      await load(path, alias, true);
    } catch (error) {
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col gap-3" aria-labelledby="sftp-heading">
      <div className="flex flex-wrap items-center gap-2">
        <h2 id="sftp-heading" className="mr-auto font-medium">{t("sftp.heading")}</h2>
        <select
          aria-label={t("sftp.host")}
          value={alias}
          onChange={(event) => setAlias(event.target.value)}
          className="rounded-md border border-control-line bg-control px-2 py-1.5 text-sm"
        >
          <option value="" disabled>{t(aliases.length === 0 ? "sftp.noHosts" : "sftp.chooseHost")}</option>
          {aliases.map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
        <input
          aria-label={t("sftp.path")}
          value={pathDraft}
          onChange={(event) => setPathDraft(event.target.value)}
          onKeyDown={(event) => { if (event.key === "Enter" && !dirty) void load(pathDraft); }}
          className="min-w-56 grow rounded-md border border-control-line bg-control px-2 py-1.5 font-mono text-sm"
        />
        <Button disabled={busy || dirty || alias === ""} onClick={() => void load(pathDraft)}>{t("sftp.go")}</Button>
      </div>

      {problem === "" ? null : <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}

      <div className="grid min-h-0 min-w-0 flex-1 gap-3 lg:grid-cols-[minmax(18rem,0.7fr)_minmax(24rem,1.3fr)]">
        <div
          aria-label={t("sftp.dropZone")}
          className={`flex min-h-0 min-w-0 flex-col rounded-lg border bg-card transition-shadow ${dragging ? "border-accent ring-2 ring-accent/40" : "border-line"}`}
          onDragEnter={(event) => { event.preventDefault(); if (!busy && alias !== "") setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDrop={(event) => { void acceptDrop(event); }}
        >
          <div className="flex flex-wrap gap-2 border-b border-line p-2">
            <Button disabled={busy || dirty || path === "/"} onClick={() => void load(parentOf(path))}>{t("sftp.up")}</Button>
            <Button disabled={busy || alias === ""} onClick={() => void makeDirectory()}>{t("sftp.newFolder")}</Button>
            <Button disabled={busy || alias === ""} onClick={() => upload.current?.click()}>{t("sftp.upload")}</Button>
            <Button disabled={busy || alias === ""} onClick={() => folderUpload.current?.click()}>{t("sftp.uploadFolder")}</Button>
            <span className="ml-auto self-center text-xs text-ink-muted">{t(dragging ? "sftp.dropNow" : "sftp.dropHint")}</span>
            <input
              ref={upload}
              type="file"
              multiple
              className="hidden"
              onChange={(event) => {
                const files = Array.from(event.target.files ?? []).flatMap((file) => {
                  const relativePath = safeRelativePath(file.name);
                  return relativePath === null ? [] : [{ file, relativePath }];
                });
                event.target.value = "";
                void uploadFiles(files);
              }}
            />
            <input
              ref={(element) => { folderUpload.current = element; element?.setAttribute("webkitdirectory", ""); }}
              type="file"
              multiple
              className="hidden"
              onChange={(event) => {
                const files = Array.from(event.target.files ?? []).flatMap((file) => {
                  const relativePath = safeRelativePath((file as File & { webkitRelativePath?: string }).webkitRelativePath ?? file.name);
                  return relativePath === null ? [] : [{ file, relativePath }];
                });
                event.target.value = "";
                void uploadFiles(files);
              }}
            />
          </div>
          {progress === null ? null : <div className="border-b border-line px-3 py-2 text-xs"><div className="flex items-center gap-2"><progress className="min-w-24 grow" max={progress.totalBytes ?? progress.total} value={progress.totalBytes === null ? progress.processed : progress.bytes} /><span className="tabular-nums text-ink-muted">{progress.kind === "upload" ? `${progress.processed}/${progress.total} · ${progress.bytes.toLocaleString()}/${(progress.totalBytes ?? 0).toLocaleString()} B` : `${progress.bytes.toLocaleString()} B`}</span>{transferAbort.current === null ? null : <button type="button" className="text-danger" onClick={cancelTransfer}>{t("sftp.cancelTransfer")}</button>}</div></div>}
          {uploads.length === 0 ? null : <ul aria-label={t("sftp.uploads")} className="max-h-28 overflow-auto border-b border-line px-3 py-2 text-xs">{uploads.map((item) => <li key={item.id} className="flex min-w-0 items-center gap-2"><span className="min-w-0 grow truncate font-mono">{item.name}</span><span className={item.status === "failed" ? "text-danger" : item.status === "done" ? "text-live" : "text-ink-muted"}>{item.status === "failed" ? item.problem : t(`sftp.upload.${item.status}`)}</span></li>)}</ul>}
          <div className="min-h-0 min-w-0 overflow-auto">
            <table className="w-full min-w-[52rem] text-left text-sm">
              <thead className="sticky top-0 bg-card text-xs text-ink-muted"><tr>
                <SortableTableHeader column="name" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-3 py-2">{t("sftp.name")}</SortableTableHeader>
                <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="w-24 whitespace-nowrap px-3 py-2">{t("sftp.type")}</SortableTableHeader>
                <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-3 py-2 text-right" buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
                <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-3 py-2">{t("sftp.modified")}</SortableTableHeader>
                <SortableTableHeader column="mode" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-3 py-2">{t("sftp.permissions")}</SortableTableHeader>
                <th><span className="sr-only">{t("sftp.actions")}</span></th>
              </tr></thead>
              <tbody>
                {displayedEntries.map((entry) => (
                  <tr key={entry.path} className="border-t border-line">
                    <td className="max-w-48 px-3 py-2">
                      <button type="button" className="block w-full truncate text-left font-mono hover:text-accent" onClick={() => entry.type === "directory" ? void load(entry.path) : void openText(entry)}>{entry.type === "directory" ? "▸ " : ""}{entry.name}</button>
                    </td>
                    <td className="w-24 whitespace-nowrap px-3 py-2 text-xs text-ink-muted">{t(`sftp.type.${entry.type}`)}</td>
                    <td className="px-3 py-2 text-right text-xs text-ink-muted">{entry.type === "file" ? entry.size.toLocaleString() : "—"}</td>
                    <td className="whitespace-nowrap px-3 py-2 text-xs text-ink-muted">{new Date(entry.modifiedAt).toLocaleString()}</td>
                    <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-ink-muted">{entry.mode}</td>
                    <td className="whitespace-nowrap px-2 py-1 text-right">
                      {entry.type === "file" || entry.type === "directory" ? <button className="px-1 text-xs text-accent" onClick={() => void download(entry)}>{t("sftp.download")}</button> : null}
                      {entry.type === "file" || entry.type === "directory" ? <button className="px-1 text-xs text-accent" onClick={() => void chmod(entry)}>{t("sftp.chmod")}</button> : null}
                      <button className="px-1 text-xs text-accent" onClick={() => void rename(entry)}>{t("sftp.rename")}</button>
                      <button className="px-1 text-xs text-danger" onClick={() => setDeleting(entry)}>{t("sftp.delete")}</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border border-line bg-card">
          {opened === null ? (
            <div className="grid h-full min-h-64 place-items-center p-6 text-sm text-ink-muted">{t("sftp.editorEmpty")}</div>
          ) : (
            <>
              <div className="flex items-center gap-2 border-b border-line px-3 py-2">
                <code className="min-w-0 grow truncate text-xs">{opened.entry.path}</code>
                {dirty ? <span className="text-xs text-notice-ink">{t("sftp.unsaved")}</span> : null}
                <Button disabled={busy || !dirty} onClick={() => void save()}>{t("sftp.save")}</Button>
                <button type="button" className="text-xs text-ink-muted" onClick={() => { if (!dirty) setOpened(null); }}>{t("sftp.close")}</button>
              </div>
              <div className="min-h-0 flex-1">
                <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
                  <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} />
                </Suspense>
              </div>
            </>
          )}
        </div>
      </div>

      {deleting === null ? null : (
        <ConfirmDialog
          id="sftp-delete-heading"
          heading={t("sftp.deleteHeading")}
          body={<p className="break-all text-sm text-ink-muted">{deleting.path}</p>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          onConfirm={() => void remove()}
          onCancel={() => setDeleting(null)}
        />
      )}
      {overwriteDecision === null ? null : (
        <ConfirmDialog
          id="sftp-overwrite-heading"
          heading={t("sftp.overwriteHeading")}
          body={<p className="break-all text-sm text-ink-muted">{overwriteDecision.path}</p>}
          confirmLabel={t("sftp.overwrite")}
          cancelLabel={t("sftp.skip")}
          onConfirm={() => { overwriteDecision.decide(true); setOverwriteDecision(null); }}
          onCancel={() => { overwriteDecision.decide(false); setOverwriteDecision(null); }}
        />
      )}
    </section>
  );
}

import { Suspense, lazy, useEffect, useRef, useState, useSyncExternalStore, type DragEvent as ReactDragEvent, type MouseEvent as ReactMouseEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { clipboard } from "../ui/clipboard";
import { InputDialog } from "../ui/InputDialog";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import {
  compareText,
  nextSort,
  ordered,
  SortableTableHeader,
  type SortDirection,
} from "../ui/tableSort";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { sftpApi, type RemoteEntry, type RemoteTextFile } from "./api";
import { directoryPaths, safeRelativePath, symbolicModeToOctal, type LocalTransferFile } from "./transfers";
import { TransferManagerList } from "./TransferManagerList";
import { sftpTransferManager } from "./transferManager";

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

function compactSFTPViewport(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(max-width: 767px)").matches;
}

type SFTPSort = "name" | "type" | "size" | "modified";

type SFTPMenu = "create" | "selected";

type SFTPInputIntent =
  | { kind: "mkdir" }
  | { kind: "rename"; entry: RemoteEntry }
  | { kind: "chmod"; entry: RemoteEntry };

export type SFTPTarget = {
  alias: string;
  path: string;
  action: "browse" | "edit" | "download";
  request: number;
};

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

export function SFTPPanel({
  aliases,
  target = null,
  onTargetHandled = () => undefined,
}: {
  aliases: string[];
  target?: SFTPTarget | null;
  onTargetHandled?: (request: number) => void;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [path, setPath] = useState("");
  const [pathDraft, setPathDraft] = useState("");
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [opened, setOpened] = useState<RemoteTextFile | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [deleting, setDeleting] = useState<RemoteEntry[] | null>(null);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(() => new Set());
  const [navigation, setNavigation] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const [filter, setFilter] = useState("");
  const [menu, setMenu] = useState<SFTPMenu | null>(null);
  const [dragging, setDragging] = useState(false);
  const [compactViewport, setCompactViewport] = useState(compactSFTPViewport);
  const [sort, setSort] = useState<{ key: SFTPSort; direction: SortDirection }>({
    key: "name",
    direction: "ascending",
  });
  const upload = useRef<HTMLInputElement>(null);
  const folderUpload = useRef<HTMLInputElement>(null);
  const openingTarget = useRef(false);
  const handledTarget = useRef(0);
  const loadGeneration = useRef(0);
  const menuRoot = useRef<HTMLDivElement>(null);
  const menuPanel = useRef<HTMLDivElement>(null);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const selectAll = useRef<HTMLInputElement>(null);
  const selectionAnchor = useRef<string | null>(null);

  useDismissibleLayer({
    open: menu !== null,
    containerRefs: [menuRoot],
    onDismiss: () => setMenu(null),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menu !== null, menuRef: menuPanel, onClose: () => setMenu(null) });

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia("(max-width: 767px)");
    const update = () => setCompactViewport(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  const transferJobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const refreshedUploads = useRef(new Set<string>());
  const dirty = opened !== null && contents !== opened.contents;
  const sortedEntries = ordered(
    entries,
    (left, right) => {
      switch (sort.key) {
        case "type": return compareText(left.type, right.type);
        case "size": return left.size - right.size;
        case "modified": return Date.parse(left.modifiedAt) - Date.parse(right.modifiedAt);
        case "name": return compareText(left.name, right.name);
      }
    },
    sort.direction,
  );
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  const displayedEntries = normalizedFilter === ""
    ? sortedEntries
    : sortedEntries.filter((entry) => entry.name.toLocaleLowerCase().includes(normalizedFilter));
  const selectedEntries = entries.filter((entry) => selectedPaths.has(entry.path));
  const selectedEntry = selectedEntries.length === 1 ? selectedEntries[0] ?? null : null;
  const allDisplayedSelected = displayedEntries.length > 0 && displayedEntries.every((entry) => selectedPaths.has(entry.path));

  useEffect(() => {
    if (selectAll.current !== null) {
      selectAll.current.indeterminate = selectedEntries.length > 0 && !allDisplayedSelected;
    }
  }, [allDisplayedSelected, selectedEntries.length]);

  function changeSort(key: SFTPSort) {
    setSort((current) => nextSort(current.key, current.direction, key));
  }

  function selectHost(nextAlias: string) {
    // Invalidate every request started for the previous host before React runs
    // the alias effect. Keeping its rows visible would also let an action for
    // host A be submitted with host B's alias during the hand-off render.
    loadGeneration.current += 1;
    setAlias(nextAlias);
    setPath("");
    setPathDraft("");
    setEntries([]);
    setOpened(null);
    setContents("");
    setDeleting(null);
    setSelectedPaths(new Set());
    setNavigation({ paths: [], index: -1 });
    setFilter("");
    setMenu(null);
    setProblem("");
    setBusy(false);
  }

  async function load(nextPath = path, nextAlias = alias, preserveEditor = false, recordNavigation = true): Promise<RemoteEntry[] | null> {
    const generation = ++loadGeneration.current;
    if (nextAlias === "") {
      setBusy(false);
      return null;
    }
    setBusy(true);
    setProblem("");
    try {
      const listing = await sftpApi.list(nextAlias, nextPath);
      if (generation !== loadGeneration.current) return null;
      setPath(listing.path);
      setPathDraft(listing.path);
      setEntries(listing.entries);
      if (recordNavigation) {
        setNavigation((current) => {
          if (current.paths[current.index] === listing.path) return current;
          const paths = [...current.paths.slice(0, current.index + 1), listing.path];
          return { paths, index: paths.length - 1 };
        });
      }
      setSelectedPaths((current) => new Set(listing.entries.filter((entry) => current.has(entry.path)).map((entry) => entry.path)));
      setMenu(null);
      if (!preserveEditor) {
        setOpened(null);
        setContents("");
      }
      return listing.entries;
    } catch (error) {
      if (generation !== loadGeneration.current) return null;
      const code = failureCode(error);
      setProblem(code === "sftp_failed" ? t("sftp.connectionFailed") : code || (error instanceof Error ? error.message : t("sftp.connectionFailed")));
      return null;
    } finally {
      if (generation === loadGeneration.current) setBusy(false);
    }
  }

  useEffect(() => {
    if (target === null || target.request === handledTarget.current) return;
    handledTarget.current = target.request;
    onTargetHandled(target.request);
    if (!aliases.includes(target.alias) || !target.path.startsWith("/") || target.path.length > 4096 || /[\x00\r\n]/u.test(target.path)) {
      setProblem(t("sftp.linkTargetInvalid"));
      return;
    }
    openingTarget.current = true;
    selectHost(target.alias);
    const directory = parentOf(target.path);
    void load(directory, target.alias).then(async (loaded) => {
      if (loaded === null) return;
      const entry = loaded.find((candidate) => candidate.path === target.path);
      if (target.action === "browse") {
        if (entry?.type === "directory") await load(entry.path, target.alias);
        return;
      }
      if (entry === undefined) {
        setProblem(t("sftp.linkTargetNotFound"));
        return;
      }
      if (target.action === "edit") {
        if (entry.type !== "file") {
          setProblem(t("sftp.linkTargetNotFile"));
          return;
        }
        await openText(entry, target.alias);
        return;
      }
      await download(entry, target.alias);
    }).finally(() => { openingTarget.current = false; });
    // The request number makes an intentional repeat actionable while preventing route rerenders from reopening it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.request]);

  useEffect(() => {
    if (alias !== "" && !openingTarget.current) void load("", alias);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [alias]);

  useEffect(() => {
    const completed = transferJobs.filter((job) => job.direction === "upload" && job.status === "completed" && job.alias === alias && parentOf(job.remotePath) === path && !refreshedUploads.current.has(job.id));
    if (completed.length === 0) return;
    for (const job of completed) refreshedUploads.current.add(job.id);
    void load(path, alias, true);
    // load intentionally follows the current alias/path snapshot for each completed job.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transferJobs, alias, path]);

  async function openText(entry: RemoteEntry, targetAlias = alias) {
    if (dirty) {
      setProblem(t("sftp.unsavedBlocked"));
      return;
    }
    const generation = ++loadGeneration.current;
    setBusy(true);
    setProblem("");
    try {
      const file = await sftpApi.readText(targetAlias, entry.path);
      if (generation !== loadGeneration.current) return;
      setOpened(file);
      setContents(file.contents);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      const code = failureCode(error);
      if (code === "sftp_not_utf8" || code === "sftp_text_too_large") {
        setProblem(t(code === "sftp_not_utf8" ? "sftp.binaryHint" : "sftp.tooLargeHint"));
      } else {
        setProblem(code || (error instanceof Error ? error.message : "sftp_failed"));
      }
    } finally {
      if (generation === loadGeneration.current) setBusy(false);
    }
  }

  async function save() {
    if (opened === null) return;
    const generation = loadGeneration.current;
    const targetAlias = alias;
    setBusy(true);
    setProblem("");
    try {
      const saved = await sftpApi.saveText(targetAlias, opened.entry.path, contents, opened.revision);
      if (generation !== loadGeneration.current) return;
      const loaded = await load(parentOf(saved.entry.path), targetAlias, true);
      if (loaded === null) return;
      setOpened(saved);
      setContents(saved.contents);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
    } finally {
      if (generation === loadGeneration.current) setBusy(false);
    }
  }

  async function makeDirectory(name: string) {
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    setBusy(true);
    try {
      await sftpApi.mkdir(targetAlias, join(targetPath, name));
      if (generation !== loadGeneration.current) return;
      await load(targetPath, targetAlias);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function rename(entry: RemoteEntry, name: string) {
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    setBusy(true);
    try {
      await sftpApi.rename(targetAlias, entry.path, join(targetPath, name));
      if (generation !== loadGeneration.current) return;
      await load(targetPath, targetAlias);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function remove() {
    if (deleting === null) return;
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    setBusy(true);
    const failures: unknown[] = [];
    for (const entry of deleting) {
      try {
        await sftpApi.remove(targetAlias, entry.path);
      } catch (error) {
        failures.push(error);
      }
    }
    if (generation !== loadGeneration.current) return;
    setDeleting(null);
    await load(targetPath, targetAlias);
    if (failures.length > 0) {
      const first = failures[0];
      setProblem(deleting.length === 1
        ? failureCode(first) || (first instanceof Error ? first.message : "delete_failed")
        : t("sftp.deleteFailedCount", { count: failures.length }));
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
    const generation = loadGeneration.current;
    setBusy(true);
    setProblem("");
    const selections = safeFiles.map((source) => ({
      alias, remotePath: join(path, source.relativePath), localName: source.relativePath, file: source.file,
    }));
    let admission: ReturnType<typeof sftpTransferManager.reserveUploads> | undefined;
    try {
      admission = sftpTransferManager.reserveUploads(selections);
      const directories = [...new Set([...directoryPaths(safeFiles), ...safeDirectories])]
        .sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
      for (const directory of directories) {
        try {
          await sftpApi.mkdir(alias, join(path, directory));
        } catch (error) {
          if (failureCode(error) !== "sftp_exists") throw error;
        }
      }
      const folderName = [...safeDirectories, ...safeFiles.map((source) => source.relativePath)]
        .map((value) => value.split("/")[0] ?? value).find((value) => value !== "") ?? t("sftp.manager.folder");
      const folderBatch = safeDirectories.length > 0 || safeFiles.length > 1 || safeFiles.some((source) => source.relativePath.includes("/"));
      await sftpTransferManager.addUploads(selections, {
        name: folderBatch ? folderName : safeFiles[0]?.relativePath ?? folderName,
        kind: folderBatch ? "folder" : "file",
      }, admission);
      admission = undefined;
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      admission?.release();
      if (generation === loadGeneration.current) setBusy(false);
    }
  }

  async function acceptDrop(event: ReactDragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    if (busy || alias === "") return;
    const selection = await droppedFiles(event.dataTransfer);
    await uploadFiles(selection.files, selection.directories);
  }

  async function download(entry: RemoteEntry, targetAlias = alias) {
    await downloadEntries([entry], targetAlias);
  }

  async function downloadEntries(targets: RemoteEntry[], targetAlias = alias) {
    if (busy || targets.length === 0) return;
    setProblem("");
    const results = await Promise.allSettled(targets
      .filter((entry) => entry.type === "file" || entry.type === "directory")
      .map((entry) => sftpTransferManager.addDownload(targetAlias, entry.path, entry.type === "directory" ? "folder" : "file", entry.type === "file" ? entry.size : -1)));
    const failed = results.find((result) => result.status === "rejected");
    if (failed?.status === "rejected") {
      setProblem(failureCode(failed.reason) || (failed.reason instanceof Error ? failed.reason.message : "sftp_failed"));
    }
  }

  async function chmod(entry: RemoteEntry, mode: string) {
    if (entry.type === "symlink" || entry.type === "other") return;
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    setBusy(true);
    try {
      await sftpApi.chmod(targetAlias, entry.path, mode, entry.revision);
      if (generation !== loadGeneration.current) return;
      await load(targetPath, targetAlias, true);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  function activate(entry: RemoteEntry) {
    setMenu(null);
    if (entry.type === "directory") void load(entry.path);
    else void openText(entry);
  }

  function selectEntry(entry: RemoteEntry, event?: ReactMouseEvent<HTMLButtonElement>) {
    const anchorIndex = selectionAnchor.current === null
      ? -1
      : displayedEntries.findIndex((candidate) => candidate.path === selectionAnchor.current);
    const entryIndex = displayedEntries.findIndex((candidate) => candidate.path === entry.path);
    if (event?.shiftKey && anchorIndex >= 0 && entryIndex >= 0) {
      const start = Math.min(anchorIndex, entryIndex);
      const end = Math.max(anchorIndex, entryIndex);
      setSelectedPaths((current) => {
        const next = event.metaKey || event.ctrlKey ? new Set(current) : new Set<string>();
        for (const candidate of displayedEntries.slice(start, end + 1)) next.add(candidate.path);
        return next;
      });
    } else if (event?.metaKey || event?.ctrlKey) {
      toggleSelection(entry);
      selectionAnchor.current = entry.path;
      return;
    } else {
      setSelectedPaths(new Set([entry.path]));
      selectionAnchor.current = entry.path;
    }
    setMenu(null);
  }

  function toggleSelection(entry: RemoteEntry) {
    setSelectedPaths((current) => {
      const next = new Set(current);
      if (next.has(entry.path)) next.delete(entry.path);
      else next.add(entry.path);
      return next;
    });
    selectionAnchor.current = entry.path;
    setMenu(null);
  }

  function toggleAllDisplayed() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of displayedEntries) {
        if (allDisplayedSelected) next.delete(entry.path);
        else next.add(entry.path);
      }
      return next;
    });
    setMenu(null);
  }

  function selectAllDisplayed() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of displayedEntries) next.add(entry.path);
      return next;
    });
    selectionAnchor.current = displayedEntries[0]?.path ?? null;
    setMenu(null);
  }

  function invertDisplayedSelection() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of displayedEntries) {
        if (next.has(entry.path)) next.delete(entry.path);
        else next.add(entry.path);
      }
      return next;
    });
    setMenu(null);
  }

  async function copySelected(kind: "name" | "path") {
    setMenu(null);
    try {
      await clipboard.writeText(selectedEntries.map((entry) => kind === "name" ? entry.name : entry.path).join("\n"));
    } catch {
      setProblem(t("copy.refused"));
    }
  }

  async function navigateHistory(offset: -1 | 1) {
    const nextIndex = navigation.index + offset;
    const destination = navigation.paths[nextIndex];
    if (destination === undefined) return;
    const loaded = await load(destination, alias, false, false);
    if (loaded === null) return;
    setNavigation((current) => current.paths[nextIndex] === destination
      ? { ...current, index: nextIndex }
      : current);
  }

  function toggleMenu(next: SFTPMenu, trigger: HTMLButtonElement) {
    menuTrigger.current = trigger;
    setMenu((current) => current === next ? null : next);
  }

  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col gap-2" aria-labelledby="sftp-heading">
      <div className="flex flex-wrap items-center gap-2 border-b border-line pb-2">
        <h2 id="sftp-heading" className="mr-auto font-medium">{t("sftp.heading")}</h2>
        <select
          aria-label={t("sftp.host")}
          value={alias}
          onChange={(event) => selectHost(event.target.value)}
          className="rounded-md border border-control-line bg-control px-2 py-1.5 text-sm"
        >
          <option value="" disabled>{t(aliases.length === 0 ? "sftp.noHosts" : "sftp.chooseHost")}</option>
          {aliases.map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
        <div role="group" aria-label={t("sftp.navigation")} className="flex shrink-0 overflow-hidden rounded-md border border-control-line bg-control">
          <button type="button" aria-label={t("sftp.back")} disabled={busy || dirty || navigation.index <= 0} onClick={() => void navigateHistory(-1)} className="flex size-9 items-center justify-center border-r border-control-line text-ink-muted hover:bg-select-fill disabled:text-ink-faint">
            <span aria-hidden="true">←</span>
          </button>
          <button type="button" aria-label={t("sftp.forward")} disabled={busy || dirty || navigation.index < 0 || navigation.index >= navigation.paths.length - 1} onClick={() => void navigateHistory(1)} className="flex size-9 items-center justify-center border-r border-control-line text-ink-muted hover:bg-select-fill disabled:text-ink-faint">
            <span aria-hidden="true">→</span>
          </button>
          <button type="button" aria-label={t("sftp.homeDirectory")} disabled={busy || dirty || alias === ""} onClick={() => void load("")} className="flex size-9 items-center justify-center border-r border-control-line text-ink-muted hover:bg-select-fill disabled:text-ink-faint">
            <Icon name="home" className="size-4" />
          </button>
          <button type="button" aria-label={t("sftp.rootDirectory")} disabled={busy || dirty || alias === "" || path === "/"} onClick={() => void load("/")} className="flex size-9 items-center justify-center font-mono text-sm text-ink-muted hover:bg-select-fill disabled:text-ink-faint">
            /
          </button>
        </div>
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

      <div className="grid min-h-0 min-w-0 flex-1 grid-cols-1 gap-2">
        <div
          aria-label={t("sftp.dropZone")}
          className={`flex min-h-0 min-w-0 flex-col rounded border bg-card transition-shadow ${dragging ? "border-accent ring-1 ring-accent" : "border-line"}`}
          onDragEnter={(event) => { event.preventDefault(); if (!busy && alias !== "") setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDrop={(event) => { void acceptDrop(event); }}
        >
          <div ref={menuRoot} className="relative flex min-h-10 items-center gap-1 border-b border-line bg-toolbar px-2 py-1">
            <button
              type="button"
              aria-label={t("sftp.createActions")}
              aria-haspopup="menu"
              aria-expanded={menu === "create"}
              disabled={busy || alias === ""}
              onClick={(event) => toggleMenu("create", event.currentTarget)}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
            >
              <Icon name="plus" className="size-4" />
            </button>
            <label className="relative min-w-24 max-w-52 grow">
              <span className="sr-only">{t("sftp.filter")}</span>
              <Icon name="search" className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-ink-muted" />
              <input
                type="search"
                aria-label={t("sftp.filter")}
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                placeholder={t("sftp.filterPlaceholder")}
                className="h-8 w-full rounded border border-control-line bg-control py-1 pl-7 pr-2 text-xs outline-none focus:border-accent"
              />
            </label>
            <span className="hidden min-w-0 grow truncate text-xs text-ink-muted lg:block">{t(dragging ? "sftp.dropNow" : "sftp.dropHint")}</span>
            {selectedEntries.length === 0 ? null : (
              <span className="hidden max-w-40 truncate text-xs text-ink-muted sm:block">
                {selectedEntry === null
                  ? t("sftp.selectedCount", { count: selectedEntries.length })
                  : t("sftp.selected", { name: selectedEntry.name })}
              </span>
            )}
            <button
              type="button"
              aria-label={selectedEntries.length === 0
                ? t("sftp.noSelectionActions")
                : selectedEntry === null
                  ? t("sftp.selectedActionsCount", { count: selectedEntries.length })
                  : t("sftp.selectedActions", { name: selectedEntry.name })}
              aria-haspopup="menu"
              aria-expanded={menu === "selected"}
              disabled={selectedEntries.length === 0}
              onClick={(event) => toggleMenu("selected", event.currentTarget)}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
            >
              <Icon name="moreHorizontal" className="size-4" />
            </button>
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
            {menu === "create" ? (
              <div ref={menuPanel} role="menu" aria-label={t("sftp.createActions")} className="absolute left-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "mkdir" }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFolder")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); upload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.upload")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); folderUpload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.uploadFolder")}</button>
              </div>
            ) : null}
            {menu === "selected" && selectedEntries.length > 0 ? (
              <div ref={menuPanel} role="menu" aria-label={selectedEntry === null ? t("sftp.selectedActionsCount", { count: selectedEntries.length }) : t("sftp.selectedActions", { name: selectedEntry.name })} className="absolute right-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                {selectedEntry === null ? null : <button type="button" role="menuitem" disabled={busy || dirty} onClick={() => activate(selectedEntry)} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t(selectedEntry.type === "directory" ? "sftp.openFolder" : "sftp.editFile")}</button>}
                {selectedEntries.some((entry) => entry.type === "file" || entry.type === "directory") ? <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); void downloadEntries(selectedEntries); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.download")}</button> : null}
                {selectedEntry !== null && (selectedEntry.type === "file" || selectedEntry.type === "directory") ? <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "chmod", entry: selectedEntry }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.chmod")}</button> : null}
                {selectedEntry === null ? null : <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "rename", entry: selectedEntry }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.rename")}</button>}
                <button type="button" role="menuitem" onClick={() => void copySelected("name")} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t(selectedEntries.length === 1 ? "sftp.copyName" : "sftp.copyNames")}</button>
                <button type="button" role="menuitem" onClick={() => void copySelected("path")} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t(selectedEntries.length === 1 ? "sftp.copyPath" : "sftp.copyPaths")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setDeleting(selectedEntries); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm text-danger hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.delete")}</button>
                <button type="button" role="menuitem" onClick={invertDisplayedSelection} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.invertSelection")}</button>
                <button type="button" role="menuitem" onClick={() => { setMenu(null); setSelectedPaths(new Set()); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{t("sftp.clearSelection")}</button>
              </div>
            ) : null}
          </div>
          <div className="min-h-0 min-w-0 flex-1 overflow-auto" onKeyDown={(event) => {
            if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "a") {
              event.preventDefault();
              selectAllDisplayed();
            }
          }}>
            {compactViewport ? (
              <ul aria-label={t("sftp.entries")} className="divide-y divide-line">
                {path !== "" && path !== "/" ? (
                  <li>
                    <button type="button" disabled={busy || dirty} onClick={() => void load(parentOf(path))} className="flex min-h-10 w-full items-center gap-2 px-2 py-1.5 text-left text-sm hover:bg-select-fill disabled:text-ink-faint">
                      <Icon name="groups" className="size-4 text-accent" />
                      <span aria-hidden="true" className="font-mono">..</span>
                      <span className="sr-only">{t("sftp.parentDirectory")}</span>
                    </button>
                  </li>
                ) : null}
                {displayedEntries.map((entry) => (
                  <li key={entry.path} className={`flex items-center ${selectedPaths.has(entry.path) ? "bg-select-fill" : ""}`}>
                    <input
                      type="checkbox"
                      aria-label={t("sftp.selectEntry", { name: entry.name })}
                      checked={selectedPaths.has(entry.path)}
                      onChange={() => toggleSelection(entry)}
                      className="ml-2 size-4 shrink-0 accent-accent"
                    />
                    <button
                      type="button"
                      aria-label={entry.name}
                      aria-pressed={selectedPaths.has(entry.path)}
                      className="flex min-h-10 min-w-0 grow items-center gap-2 px-2 py-1.5 text-left hover:bg-select-fill"
                      onClick={(event) => selectEntry(entry, event)}
                      onDoubleClick={() => activate(entry)}
                      onKeyDown={(event) => { if (event.key === "Enter") activate(entry); }}
                    >
                      <Icon name={entry.type === "directory" ? "groups" : entry.type === "symlink" ? "chevronRight" : "config"} className={`size-4 ${entry.type === "directory" ? "text-accent" : "text-ink-muted"}`} />
                      <span className="min-w-0 grow">
                        <span className="block truncate font-mono text-sm font-medium leading-4 text-ink">{entry.name}</span>
                        <span className="mt-0.5 flex min-w-0 gap-2 text-[11px] leading-3 text-ink-muted">
                          <span className="font-mono">{entry.mode}</span>
                          <span>{entry.type === "file" ? entry.size.toLocaleString() : "—"}</span>
                          <time className="truncate" dateTime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleString()}</time>
                        </span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
            <table className="w-full min-w-[44rem] text-left text-sm">
              <thead className="sticky top-0 bg-toolbar text-xs text-ink-muted"><tr>
                <th scope="col" className="w-9 px-2 py-1.5">
                  <input
                    ref={selectAll}
                    type="checkbox"
                    aria-label={t("sftp.selectAll")}
                    checked={allDisplayedSelected}
                    onChange={toggleAllDisplayed}
                    className="size-4 accent-accent"
                  />
                </th>
                <SortableTableHeader column="name" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5">{t("sftp.name")}</SortableTableHeader>
                <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5">{t("sftp.modified")}</SortableTableHeader>
                <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5 text-right" buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
                <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="w-24 whitespace-nowrap px-2 py-1.5">{t("sftp.type")}</SortableTableHeader>
              </tr></thead>
              <tbody>
                {path !== "" && path !== "/" ? (
                  <tr className="border-t border-line hover:bg-select-fill">
                    <td className="px-2 py-1" colSpan={5}>
                      <button type="button" disabled={busy || dirty} onClick={() => void load(parentOf(path))} className="flex w-full items-center gap-2 rounded py-0.5 text-left text-sm disabled:text-ink-faint">
                        <Icon name="groups" className="size-4 text-accent" />
                        <span aria-hidden="true" className="font-mono">..</span>
                        <span className="sr-only">{t("sftp.parentDirectory")}</span>
                      </button>
                    </td>
                  </tr>
                ) : null}
                {displayedEntries.map((entry) => (
                  <tr key={entry.path} aria-selected={selectedPaths.has(entry.path)} onDoubleClick={() => activate(entry)} className={`cursor-default border-t border-line ${selectedPaths.has(entry.path) ? "bg-select-fill" : "hover:bg-select-fill"}`}>
                    <td className="w-9 px-2 py-1">
                      <input
                        type="checkbox"
                        aria-label={t("sftp.selectEntry", { name: entry.name })}
                        checked={selectedPaths.has(entry.path)}
                        onChange={() => toggleSelection(entry)}
                        onDoubleClick={(event) => event.stopPropagation()}
                        className="size-4 accent-accent"
                      />
                    </td>
                    <td className="max-w-64 px-2 py-1">
                      <button type="button" aria-label={entry.name} aria-pressed={selectedPaths.has(entry.path)} onClick={(event) => selectEntry(entry, event)} className="flex w-full min-w-0 items-center gap-2 rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent" onKeyDown={(event) => { if (event.key === "Enter") activate(entry); }}>
                        <Icon name={entry.type === "directory" ? "groups" : entry.type === "symlink" ? "chevronRight" : "config"} className={`size-4 ${entry.type === "directory" ? "text-accent" : "text-ink-muted"}`} />
                        <span className="min-w-0">
                          <span className="block truncate font-mono text-sm leading-4">{entry.name}</span>
                          <span className="block font-mono text-[10px] leading-3 text-ink-muted">{entry.mode}</span>
                        </span>
                      </button>
                    </td>
                    <td className="whitespace-nowrap px-2 py-1 text-xs text-ink-muted">{new Date(entry.modifiedAt).toLocaleString()}</td>
                    <td className="px-2 py-1 text-right text-xs text-ink-muted">{entry.type === "file" ? entry.size.toLocaleString() : "—"}</td>
                    <td className="w-24 whitespace-nowrap px-2 py-1 text-xs text-ink-muted">{t(`sftp.type.${entry.type}`)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            )}
          </div>
          <TransferManagerList />
        </div>

      </div>

      {opened === null ? null : (
        <ModalShell
          labelledBy="sftp-editor-heading"
          onDismiss={() => {
            if (dirty) setProblem(t("sftp.unsavedBlocked"));
            else setOpened(null);
          }}
          panelClassName="flex h-[min(52rem,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-lg"
        >
          <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
            <h2 id="sftp-editor-heading" className="min-w-0 grow truncate font-mono text-xs">{opened.entry.path}</h2>
            {dirty ? <span className="text-xs text-notice-ink">{t("sftp.unsaved")}</span> : null}
            <Button disabled={busy || !dirty} onClick={() => void save()}>{t("sftp.save")}</Button>
            <button type="button" disabled={dirty} className="text-xs text-ink-muted disabled:text-ink-faint" onClick={() => setOpened(null)}>{t("sftp.close")}</button>
          </div>
          <div className="min-h-0 flex-1">
            <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
              <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} />
            </Suspense>
          </div>
        </ModalShell>
      )}

      {deleting === null ? null : (
        <ConfirmDialog
          id="sftp-delete-heading"
          heading={deleting.length === 1 ? t("sftp.deleteHeading") : t("sftp.deleteHeadingCount", { count: deleting.length })}
          body={<ul className="max-h-48 space-y-1 overflow-auto text-sm text-ink-muted">
            {deleting.map((entry) => <li key={entry.path} className="break-all font-mono">{entry.path}</li>)}
          </ul>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          onConfirm={() => void remove()}
          onCancel={() => setDeleting(null)}
        />
      )}
      {inputIntent === null ? null : (
        <InputDialog
          id="sftp-input-heading"
          heading={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "rename" ? "sftp.rename" : "sftp.chmod")}
          label={t(inputIntent.kind === "chmod" ? "sftp.chmodPrompt" : inputIntent.kind === "rename" ? "sftp.renamePrompt" : "sftp.mkdirPrompt")}
          initialValue={inputIntent.kind === "mkdir" ? "" : inputIntent.kind === "rename" ? inputIntent.entry.name : symbolicModeToOctal(inputIntent.entry.mode)}
          inputMode={inputIntent.kind === "chmod" ? "numeric" : "text"}
          submitLabel={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "rename" ? "sftp.rename" : "sftp.chmod")}
          cancelLabel={t("sftp.cancel")}
          validate={(value) => {
            if (inputIntent.kind === "chmod") return /^0?[0-7]{3}$/.test(value) ? "" : t("sftp.chmodInvalid");
            if (value === "") return t("sftp.nameRequired");
            if (value.includes("/")) return t("sftp.nameInvalid");
            if (inputIntent.kind === "rename" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            return "";
          }}
          onSubmit={(value) => {
            const intent = inputIntent;
            setInputIntent(null);
            if (intent.kind === "mkdir") void makeDirectory(value);
            else if (intent.kind === "rename") void rename(intent.entry, value);
            else void chmod(intent.entry, value);
          }}
          onCancel={() => setInputIntent(null)}
        />
      )}
    </section>
  );
}

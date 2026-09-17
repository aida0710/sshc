import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type DragEvent as ReactDragEvent,
} from "react";
import { failureCode } from "../api/client";
import type { HostEntry } from "../api/config";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { clipboard } from "../ui/clipboard";
import { InputDialog } from "../ui/InputDialog";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import { nextSort } from "../ui/tableSort";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { mobileViewportQuery, useCompactViewport, useMediaQuery } from "../ui/useMediaQuery";
import { sftpApi, type RemoteEntry } from "./api";
import { formatBytes } from "./format";
import { SFTPDetailsDialog } from "./SFTPDetailsDialog";
import { parentRowKey, SFTPEntryList, sortEntries, useSFTPEntryList, type SFTPSort, type SFTPSortState } from "./SFTPEntryList";
import { directoryPaths, remoteEntriesMime, safeRelativePath, symbolicModeToOctal, type LocalTransferFile, type RemoteDragPayload } from "./transfers";
import { TransferManagerList } from "./TransferManagerList";
import { sftpTransferManager } from "./transferManager";
import { SFTPHostPicker } from "./SFTPHostPicker";
import { SFTPNavigationControls } from "./SFTPNavigationControls";
import { SFTPTextEditor, useSFTPTextEditor } from "./SFTPTextEditor";

const noHosts: HostEntry[] = [];
function parentOf(remotePath: string): string {
  if (remotePath === "/") return "/";
  const pieces = remotePath.split("/").filter(Boolean);
  pieces.pop();
  return `/${pieces.join("/")}` || "/";
}

function join(parent: string, name: string): string {
  return `${parent === "/" ? "" : parent}/${name}`;
}

export type { SFTPSort, SFTPSortState } from "./SFTPEntryList";

// The overflow button and the row context menu open the same list of actions.
// Anchoring them to one shape keeps a right click from offering less than the
// three-dot button placed above the same rows.
type SFTPMenu =
  | { kind: "folder" }
  | { kind: "create" }
  | { kind: "selected" }
  | { kind: "context"; x: number; y: number };

type SFTPMenuAction = {
  key: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  run: () => void;
};

type SFTPInputIntent =
  | { kind: "mkdir" }
  | { kind: "createFile" }
  | { kind: "duplicate"; entry: RemoteEntry }
  | { kind: "moveTo"; entries: RemoteEntry[] }
  | { kind: "rename"; entry: RemoteEntry }
  | { kind: "chmod"; entry: RemoteEntry; recursive: boolean };

const contextMenuWidth = 224;
const contextMenuItemHeight = 40;

function MenuActionList({ actions }: { actions: SFTPMenuAction[] }) {
  return (
    <>
      {actions.map((action) => (
        <button
          key={action.key}
          type="button"
          role="menuitem"
          disabled={action.disabled === true}
          onClick={action.run}
          className={`block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0 ${action.danger === true ? "text-danger" : ""}`}
        >
          {action.label}
        </button>
      ))}
    </>
  );
}

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
  hosts = noHosts,
  target = null,
  initialLocation = null,
  initialSort = { key: "name", direction: "ascending" },
  showTransfers = true,
  onTargetHandled = () => undefined,
  onLocationChange = () => undefined,
  onSortChange = () => undefined,
  onNavigationBlockerChange,
  onDirtyChange,
  onNavigateLocation,
  onOpenTerminal,
  onQueueOpen,
  downloadLocalPath,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  // Where a restored tab should reopen. Applied once, when the declared
  // aliases have arrived and can vouch for the host.
  initialLocation?: { alias: string; path: string } | null;
  initialSort?: SFTPSortState;
  showTransfers?: boolean;
  onTargetHandled?: (request: number) => void;
  onLocationChange?: (alias: string, path: string) => void;
  onSortChange?: (sort: SFTPSortState) => void;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onDirtyChange?: ((path: string | null) => void) | undefined;
  onNavigateLocation?: ((url: string) => void) | undefined;
  onOpenTerminal?: ((alias: string, path: string) => void | Promise<void>) | undefined;
  onQueueOpen?: () => void;
  downloadLocalPath?: string | null;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [path, setPath] = useState("");
  const [connected, setConnected] = useState(false);
  const [pathDraft, setPathDraft] = useState("");
  const [pathEditing, setPathEditing] = useState(false);
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [browsing, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [deleting, setDeleting] = useState<RemoteEntry[] | null>(null);
  const [details, setDetails] = useState<RemoteEntry[] | null>(null);
  // While a search is showing, the list is its results rather than one
  // directory. Everything downstream reads listedEntries, not entries.
  const [search, setSearch] = useState<{ root: string; query: string; entries: RemoteEntry[]; truncated: boolean } | null>(null);
  // What the last change was, and how to put it back. Delete is absent on
  // purpose: SFTP has no trash, so an "undo" there would be a lie.
  const [undo, setUndo] = useState<{ label: string; run: () => Promise<void> } | null>(null);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  const [navigation, setNavigation] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const [filter, setFilter] = useState("");
  const [menu, setMenu] = useState<SFTPMenu | null>(null);
  const [dragging, setDragging] = useState(false);
  const [remoteDrop, setRemoteDrop] = useState<RemoteDragPayload | null>(null);
  const [sort, setSort] = useState<SFTPSortState>(initialSort);
  const upload = useRef<HTMLInputElement>(null);
  const folderUpload = useRef<HTMLInputElement>(null);
  const pathInput = useRef<HTMLInputElement>(null);
  const panelRoot = useRef<HTMLElement>(null);
  const compactViewport = useCompactViewport(panelRoot);
  const mobileInteraction = useMediaQuery(mobileViewportQuery);
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const headingId = useId();
  const openingTarget = useRef(false);
  const handledTarget = useRef(0);
  const openedInitialLocation = useRef(false);
  const reportLocation = useRef(onLocationChange);
  reportLocation.current = onLocationChange;
  const loadGeneration = useRef(0);
  const menuRoot = useRef<HTMLDivElement>(null);
  const menuPanel = useRef<HTMLDivElement>(null);
  const menuTrigger = useRef<HTMLButtonElement>(null);

  useDismissibleLayer({
    open: menu !== null && !mobileInteraction,
    containerRefs: [menuRoot],
    onDismiss: () => setMenu(null),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menu !== null && !mobileInteraction, menuRef: menuPanel, onClose: () => setMenu(null) });

  useEffect(() => {
    if (!pathEditing) return;
    pathInput.current?.focus();
    pathInput.current?.select();
  }, [pathEditing]);
  useEffect(() => {
    if (mobileSearchOpen) searchInput.current?.focus();
  }, [mobileSearchOpen]);

  const transferJobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const refreshedUploads = useRef(new Set<string>());
  const refreshedDeletes = useRef(new Set<string>());
  const [openQueueRequest, setOpenQueueRequest] = useState(0);
  const editor = useSFTPTextEditor({
    onProblem: setProblem,
    // The saved revision is what the listing must show next.
    onSaved: async (targetAlias, saved) => (await load(parentOf(saved.entry.path), targetAlias, true)) !== null,
    onNavigationBlockerChange,
    onDirtyChange,
    onNavigateLocation,
  });
  const dirty = editor.dirty;
  const busy = browsing || editor.busy;
  const listedEntries = search === null ? entries : search.entries;
  const sortedEntries = sortEntries(listedEntries, sort);
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  // The filter box is the query in search mode; matching again locally would
  // hide results whose match is in a parent directory's name.
  const displayedEntries = normalizedFilter === "" || search !== null
    ? sortedEntries
    : sortedEntries.filter((entry) => entry.name.toLocaleLowerCase().includes(normalizedFilter));
  const parentRowVisible = search === null && path !== "" && path !== "/";
  const list = useSFTPEntryList({
    entries: displayedEntries,
    loadedEntries: listedEntries,
    parentRowVisible,
    busy,
    locked: dirty,
    mobileInteraction,
    onActivate: (entry) => {
      if (entry.type === "directory") void load(entry.path);
      else setDetails([entry]);
    },
    onOpenParent: () => { void load(parentOf(path)); },
    onInteract: () => setMenu(null),
    onContextMenu: (_entry, x, y) => {
      menuTrigger.current = null;
      setMenu({ kind: "context", x, y });
    },
    onRenameKey: renameSelection,
    onDeleteKey: deleteSelection,
    onEscape: search === null ? undefined : endSearch,
  });
  const {
    selectedPaths, setSelectedPaths, selectedEntries, selectedEntry, rowKeys, setFocusedKey,
    pendingFocus, selectionAnchor, activeRow, activate, openParent, invertDisplayedSelection, selectAllDisplayed,
  } = list;
  // A listing that failed says so where the rows would be, with the retry next
  // to it. Repeating the same sentence in the banner above would be two voices
  // for one fact.
  const listingFailed = problem !== "" && alias !== "" && entries.length === 0;

  function changeSort(key: SFTPSort) {
    setSort((current) => {
      const next = nextSort(current.key, current.direction, key);
      onSortChange(next);
      return next;
    });
  }

  function selectHost(nextAlias: string) {
    // Invalidate every request started for the previous host before React runs
    // the alias effect. Keeping its rows visible would also let an action for
    // host A be submitted with host B's alias during the hand-off render.
    loadGeneration.current += 1;
    reportLocation.current(nextAlias, "");
    setAlias(nextAlias);
    setConnected(false);
    setPath("");
    setPathDraft("");
    setEntries([]);
    editor.close();
    setDeleting(null);
    setDetails(null);
    setSelectedPaths(new Set());
    setNavigation({ paths: [], index: -1 });
    setFilter("");
    setSearch(null);
    setMenu(null);
    setFocusedKey(null);
    setProblem("");
    setUndo(null);
    setBusy(false);
    setPendingPath(null);
    setMobileSearchOpen(false);
  }

  async function load(nextPath = path, nextAlias = alias, preserveEditor = false, recordNavigation = true): Promise<RemoteEntry[] | null> {
    const generation = ++loadGeneration.current;
    if (nextAlias === "") {
      setBusy(false);
      return null;
    }
    setBusy(true);
    setPendingPath(nextPath);
    setMenu(null);
    setProblem("");
    try {
      const listing = await sftpApi.list(nextAlias, nextPath);
      if (generation !== loadGeneration.current) return null;
      setPath(listing.path);
      setConnected(true);
      setPathDraft(listing.path);
      setPathEditing(false);
      setEntries(listing.entries);
      reportLocation.current(nextAlias, listing.path);
      if (recordNavigation) {
        setNavigation((current) => {
          if (current.paths[current.index] === listing.path) return current;
          const paths = [...current.paths.slice(0, current.index + 1), listing.path];
          return { paths, index: paths.length - 1 };
        });
      }
      setSelectedPaths((current) => new Set(listing.entries.filter((entry) => current.has(entry.path)).map((entry) => entry.path)));
      setMenu(null);
      setSearch(null);
      if (nextPath !== path || nextAlias !== alias) setUndo(null);
      if (!preserveEditor) editor.close();
      return listing.entries;
    } catch (error) {
      if (generation !== loadGeneration.current) return null;
      const code = failureCode(error);
      setProblem(code === "sftp_failed" ? t("sftp.connectionFailed") : code || (error instanceof Error ? error.message : t("sftp.connectionFailed")));
      return null;
    } finally {
      if (generation === loadGeneration.current) { setBusy(false); setPendingPath(null); }
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
        await editor.open(target.alias, entry);
        return;
      }
      await download(entry, target.alias);
    }).finally(() => { openingTarget.current = false; });
    // The request number makes an intentional repeat actionable while preventing route rerenders from reopening it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.request]);

  useEffect(() => {
    if (initialLocation === null || openedInitialLocation.current) return;
    if (!aliases.includes(initialLocation.alias) || (initialLocation.path !== "" && !initialLocation.path.startsWith("/"))) return;
    openedInitialLocation.current = true;
    loadGeneration.current += 1;
    setAlias(initialLocation.alias);
    setPath(initialLocation.path);
    setPathDraft(initialLocation.path);
    setConnected(false);
    // Restored tabs remember where they were, but never open an SSH connection
    // until the user explicitly presses Connect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aliases, initialLocation?.alias, initialLocation?.path]);

  useEffect(() => {
    if (!connected) return;
    const completed = transferJobs.filter((job) => (job.direction === "upload" || job.operation === "put") && job.status === "completed" && job.alias === alias && parentOf(job.remotePath) === path && !refreshedUploads.current.has(job.id));
    if (completed.length === 0) return;
    for (const job of completed) refreshedUploads.current.add(job.id);
    void load(path, alias, true);
    // load intentionally follows the current alias/path snapshot for each completed job.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transferJobs, alias, path, connected]);

  useEffect(() => {
    if (!connected || dirty) return;
    const completed = transferJobs.filter((job) => job.direction === "remote" && job.operation === "delete" && job.status === "completed" && job.alias === alias &&
      (search !== null ? search.root === "/" || job.remotePath === search.root || job.remotePath.startsWith(`${search.root}/`) : parentOf(job.remotePath) === path) &&
      !refreshedDeletes.current.has(job.id));
    if (completed.length === 0) return;
    for (const job of completed) refreshedDeletes.current.add(job.id);
    void refreshAfterChange(path, alias);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transferJobs, alias, path, connected, dirty, search]);

  async function makeDirectory(name: string) {
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    setBusy(true);
    try {
      await sftpApi.mkdir(targetAlias, join(targetPath, name));
      if (generation !== loadGeneration.current) return;
      pendingFocus.current = join(targetPath, name);
      await load(targetPath, targetAlias);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function makeEmptyFile(name: string) {
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    const createdPath = join(targetPath, name);
    setBusy(true);
    setProblem("");
    try {
      await sftpApi.createEmptyFile(targetAlias, createdPath);
      if (generation !== loadGeneration.current) return;
      pendingFocus.current = createdPath;
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
    const targetPath = parentOf(entry.path);
    const renamed = join(targetPath, name);
    setBusy(true);
    try {
      await sftpApi.rename(targetAlias, entry.path, renamed);
      if (generation !== loadGeneration.current) return;
      pendingFocus.current = renamed;
      await refreshAfterChange(targetPath, targetAlias);
      offerUndo(t("sftp.renamedTo", { name }), async () => {
        await sftpApi.rename(targetAlias, renamed, entry.path);
        pendingFocus.current = entry.path;
        await refreshAfterChange(targetPath, targetAlias);
      });
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function remove() {
    if (deleting === null) return;
    const existingJobIds = new Set(sftpTransferManager.getSnapshot().map((job) => job.id));
    setBusy(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(deleting.map((entry) => ({
        sourceAlias: alias, sourcePath: entry.path,
        targetAlias: alias, targetPath: entry.path,
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name, totalBytes: -1,
      })), "delete");
      setDeleting(null);
      setSelectedPaths(new Set());
      setOpenQueueRequest((current) => current + 1);
      onQueueOpen?.();
    } catch (error) {
      if (sftpTransferManager.getSnapshot().some((job) => !existingJobIds.has(job.id) && job.operation === "delete")) {
        setDeleting(null);
        setSelectedPaths(new Set());
        setOpenQueueRequest((current) => current + 1);
        onQueueOpen?.();
      }
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "delete_failed"));
    } finally {
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
    if (busy || alias === "" || !connected) return;
    const remote = typeof event.dataTransfer.getData === "function"
      ? event.dataTransfer.getData(remoteEntriesMime)
      : "";
    if (remote !== "") {
      try {
        const parsed = JSON.parse(remote) as RemoteDragPayload;
        if (typeof parsed.alias === "string" && parsed.alias !== "" && Array.isArray(parsed.entries) && parsed.entries.length > 0 &&
            parsed.entries.every((entry) => typeof entry.name === "string" && typeof entry.path === "string" &&
              (entry.type === "file" || entry.type === "directory") && typeof entry.size === "number")) {
          setRemoteDrop(parsed);
        }
      } catch {
        setProblem(t("sftp.remoteDropInvalid"));
      }
      return;
    }
    const selection = await droppedFiles(event.dataTransfer);
    await uploadFiles(selection.files, selection.directories);
  }

  function beginRemoteDrag(event: ReactDragEvent<HTMLElement>, entry: RemoteEntry) {
    if (entry.type !== "file" && entry.type !== "directory") {
      event.preventDefault();
      return;
    }
    const selected = selectedPaths.has(entry.path) ? selectedEntries : [entry];
    const payload: RemoteDragPayload = {
      alias,
      entries: selected.filter((candidate) => candidate.type === "file" || candidate.type === "directory")
        .map((candidate) => ({ name: candidate.name, path: candidate.path, type: candidate.type, size: candidate.size })),
    };
    event.dataTransfer.effectAllowed = "copyMove";
    event.dataTransfer.setData(remoteEntriesMime, JSON.stringify(payload));
    event.dataTransfer.setData("text/plain", payload.entries.map((candidate) => `${alias}:${candidate.path}`).join("\n"));
  }

  async function acceptRemoteDrop(operation: "copy" | "move") {
    const payload = remoteDrop;
    setRemoteDrop(null);
    if (payload === null || alias === "" || path === "") return;
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(payload.entries.map((entry) => ({
        sourceAlias: payload.alias,
        sourcePath: entry.path,
        targetAlias: alias,
        targetPath: join(path, entry.name),
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name,
        totalBytes: entry.type === "file" ? entry.size : -1,
      })), operation);
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    }
  }

  async function download(entry: RemoteEntry, targetAlias = alias) {
    await downloadEntries([entry], targetAlias);
  }

  async function downloadEntries(targets: RemoteEntry[], targetAlias = alias) {
    if (busy || targets.length === 0) return;
    setProblem("");
    const results = await Promise.allSettled(targets
      .filter((entry) => entry.type === "file" || entry.type === "directory")
      .map((entry) => {
        const kind = entry.type === "directory" ? "folder" : "file";
        const size = entry.type === "file" ? entry.size : -1;
        return downloadLocalPath === null || downloadLocalPath === undefined
          ? sftpTransferManager.addDownload(targetAlias, entry.path, kind, size)
          : sftpTransferManager.addRemoteTransfers([{ sourceAlias: targetAlias, sourcePath: entry.path, targetAlias,
              targetPath: `${downloadLocalPath.replace(/\/$/, "")}/${entry.name}`, name: entry.name, kind, totalBytes: size }], "get");
      }));
    const failed = results.find((result) => result.status === "rejected");
    if (failed?.status === "rejected") {
      setProblem(failureCode(failed.reason) || (failed.reason instanceof Error ? failed.reason.message : "sftp_failed"));
    }
  }

  async function chmod(entry: RemoteEntry, mode: string, recursive: boolean) {
    if (entry.type === "symlink" || entry.type === "other") return;
    const generation = loadGeneration.current;
    const targetAlias = alias;
    const targetPath = path;
    const previous = symbolicModeToOctal(entry.mode);
    setBusy(true);
    try {
      await sftpApi.chmod(targetAlias, entry.path, mode, entry.revision, recursive);
      if (generation !== loadGeneration.current) return;
      const reloaded = await load(targetPath, targetAlias, true);
      const current = reloaded?.find((candidate) => candidate.path === entry.path);
      if (!recursive && current !== undefined && previous !== mode) {
        offerUndo(t("sftp.permissionsChanged", { mode }), async () => {
          await sftpApi.chmod(targetAlias, entry.path, previous, current.revision, false);
          await load(targetPath, targetAlias, true);
        });
      }
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  // Opening a file lands on the details dialog rather than the editor. The
  // dialog is where a preview, the properties and "edit" now live together.
  async function runSearch(query = filter, root = search?.root ?? path) {
    const needle = query.trim();
    if (alias === "" || needle === "" || root === "") return;
    const generation = ++loadGeneration.current;
    setBusy(true);
    setPendingPath(root);
    setProblem("");
    try {
      const found = await sftpApi.search(alias, root, needle);
      if (generation !== loadGeneration.current) return;
      setSearch({ root: found.path, query: found.query, entries: found.entries, truncated: found.truncated });
      setSelectedPaths(new Set());
      selectionAnchor.current = null;
      setFocusedKey(null);
      setMenu(null);
      setUndo(null);
    } catch (error) {
      if (generation !== loadGeneration.current) return;
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      if (generation === loadGeneration.current) { setBusy(false); setPendingPath(null); }
    }
  }

  function endSearch() {
    if (search === null) return;
    const root = search.root;
    setSearch(null);
    setFilter("");
    void load(root);
  }

  // A rename or a delete made from the results has to be reflected there;
  // reloading the directory the user is not looking at would be no answer.
  function refreshAfterChange(targetPath: string, targetAlias: string): Promise<unknown> {
    if (search !== null) return runSearch(search.query, search.root);
    return load(targetPath, targetAlias);
  }

  function refreshCurrentDirectory() {
    if (busy || dirty || !connected) return;
    void (search !== null ? runSearch(search.query, search.root) : load(path, alias, true, false));
  }

  // The offer stands until the next thing happens. A timer would take it away
  // exactly while the user is deciding whether they meant it.
  function offerUndo(label: string, run: () => Promise<void>) {
    setUndo({
      label,
      run: async () => {
        setUndo(null);
        try {
          await run();
        } catch (error) {
          setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
        }
      },
    });
  }

  function showDetails() {
    if (selectedEntries.length === 0) return;
    setMenu(null);
    setDetails(selectedEntries);
  }

  function deleteSelection() {
    if (selectedEntries.length === 0 || busy) return;
    // Focus survives the reload by moving to whatever takes the topmost
    // removed row's place.
    const removed = new Set(selectedEntries.map((entry) => entry.path));
    const survivor = rowKeys.slice(rowKeys.findIndex((key) => removed.has(key)) + 1).find((key) => !removed.has(key));
    pendingFocus.current = survivor ?? rowKeys.filter((key) => !removed.has(key)).pop() ?? parentRowKey;
    setMenu(null);
    setDeleting(selectedEntries);
  }

  function renameSelection() {
    if (selectedEntry === null || busy) return;
    setMenu(null);
    setInputIntent({ kind: "rename", entry: selectedEntry });
  }

  async function copySelected(kind: "name" | "path") {
    setMenu(null);
    try {
      await clipboard.writeText(selectedEntries.map((entry) => kind === "name" ? entry.name : entry.path).join("\n"));
    } catch {
      setProblem(t("copy.refused"));
    }
  }

  async function copyCurrentPath() {
    try { await clipboard.writeText(path); setProblem(""); }
    catch { setProblem(t("copy.refused")); }
  }

  async function queueRemoteOperation(
    entries: RemoteEntry[],
    operation: "copy" | "move",
    destination: (entry: RemoteEntry) => string,
  ) {
    setBusy(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(entries.map((entry) => ({
        sourceAlias: alias,
        sourcePath: entry.path,
        targetAlias: alias,
        targetPath: destination(entry),
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name,
        totalBytes: entry.type === "file" ? entry.size : -1,
      })), operation);
      setSelectedPaths(new Set());
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      setBusy(false);
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

  function toggleMenu(kind: "folder" | "create" | "selected", trigger: HTMLButtonElement) {
    menuTrigger.current = trigger;
    setMenu((current) => current?.kind === kind ? null : { kind });
  }

  function selectedMenuActions(): SFTPMenuAction[] {
    const actions: SFTPMenuAction[] = [];
    if (selectedEntry !== null && selectedEntry.type === "directory") {
      actions.push({
        key: "open",
        label: t("sftp.openFolder"),
        disabled: busy || dirty,
        run: () => activate(selectedEntry),
      });
    }
    if (search !== null && selectedEntry !== null) {
      actions.push({
        key: "reveal",
        label: t("sftp.revealInFolder"),
        disabled: busy || dirty,
        run: () => {
          setMenu(null);
          pendingFocus.current = selectedEntry.path;
          void load(parentOf(selectedEntry.path));
        },
      });
    }
    actions.push({ key: "details", label: t("sftp.details"), disabled: busy, run: showDetails });
    if (selectedEntry !== null && selectedEntry.type !== "directory") {
      actions.push({
        key: "edit",
        label: t("sftp.editFile"),
        disabled: busy || dirty,
        run: () => { setMenu(null); void editor.open(alias, selectedEntry); },
      });
    }
    if (selectedEntries.some((entry) => entry.type === "file" || entry.type === "directory")) {
      actions.push({
        key: "download",
        label: t("sftp.download"),
        disabled: busy,
        run: () => { setMenu(null); void downloadEntries(selectedEntries); },
      });
    }
    if (selectedEntry !== null && (selectedEntry.type === "file" || selectedEntry.type === "directory")) {
      actions.push({
        key: "chmod",
        label: t("sftp.chmod"),
        disabled: busy,
        run: () => { setMenu(null); setInputIntent({ kind: "chmod", entry: selectedEntry, recursive: false }); },
      });
    }
    if (selectedEntry !== null && selectedEntry.type === "directory") {
      actions.push({
        key: "chmodRecursive",
        label: t("sftp.chmodRecursive"),
        disabled: busy,
        run: () => { setMenu(null); setInputIntent({ kind: "chmod", entry: selectedEntry, recursive: true }); },
      });
    }
    if (selectedEntry !== null) {
      actions.push({ key: "rename", label: t("sftp.rename"), disabled: busy, run: renameSelection });
    }
    if (selectedEntry !== null && (selectedEntry.type === "file" || selectedEntry.type === "directory")) {
      actions.push({
        key: "duplicate",
        label: t("sftp.duplicate"),
        disabled: busy,
        run: () => { setMenu(null); setInputIntent({ kind: "duplicate", entry: selectedEntry }); },
      });
    }
    if (selectedEntries.length > 0 && selectedEntries.every((entry) => entry.type === "file" || entry.type === "directory")) {
      actions.push({
        key: "moveTo",
        label: t("sftp.moveTo"),
        disabled: busy,
        run: () => { setMenu(null); setInputIntent({ kind: "moveTo", entries: selectedEntries }); },
      });
    }
    actions.push({
      key: "copyName",
      label: t(selectedEntries.length === 1 ? "sftp.copyName" : "sftp.copyNames"),
      run: () => void copySelected("name"),
    });
    actions.push({
      key: "copyPath",
      label: t(selectedEntries.length === 1 ? "sftp.copyPath" : "sftp.copyPaths"),
      run: () => void copySelected("path"),
    });
    actions.push({ key: "delete", label: t("sftp.delete"), danger: true, disabled: busy, run: deleteSelection });
    actions.push({ key: "invert", label: t("sftp.invertSelection"), run: invertDisplayedSelection });
    actions.push({
      key: "clear",
      label: t("sftp.clearSelection"),
      run: () => { setMenu(null); setSelectedPaths(new Set()); },
    });
    return actions;
  }

  function selectionMenuLabel(): string {
    return selectedEntry === null
      ? t("sftp.selectedActionsCount", { count: selectedEntries.length })
      : t("sftp.selectedActions", { name: selectedEntry.name });
  }

  function folderMenuActions(): SFTPMenuAction[] {
    const navigate = (destination: string) => { setMenu(null); void load(destination); };
    return [
      { key: "copyCurrentPath", label: t("sftp.copyPath"), disabled: !connected, run: () => { setMenu(null); void copyCurrentPath(); } },
      { key: "newFolder", label: t("sftp.newFolder"), disabled: busy || !connected, run: () => { setMenu(null); setInputIntent({ kind: "mkdir" }); } },
      { key: "newFile", label: t("sftp.newFile"), disabled: busy || !connected, run: () => { setMenu(null); setInputIntent({ kind: "createFile" }); } },
      { key: "upload", label: t("sftp.upload"), disabled: busy || !connected, run: () => { setMenu(null); upload.current?.click(); } },
      { key: "uploadFolder", label: t("sftp.uploadFolder"), disabled: busy || !connected, run: () => { setMenu(null); folderUpload.current?.click(); } },
      { key: "forward", label: t("sftp.forward"), disabled: busy || dirty || navigation.index < 0 || navigation.index >= navigation.paths.length - 1, run: () => { setMenu(null); void navigateHistory(1); } },
      { key: "home", label: t("sftp.homeDirectory"), disabled: busy || dirty || !connected, run: () => navigate("") },
      { key: "root", label: t("sftp.rootDirectory"), disabled: busy || dirty || !connected || path === "/", run: () => navigate("/") },
      ...(onOpenTerminal === undefined ? [] : [{ key: "terminal", label: t("sftp.openTerminalHere"), disabled: busy || dirty || !connected, run: () => { setMenu(null); void onOpenTerminal(alias, path); } }]),
      { key: "selectAll", label: t("sftp.selectAll"), disabled: busy || displayedEntries.length === 0, run: selectAllDisplayed },
      ...(["name", "type", "size", "modified"] as const).map((key) => ({
        key: `sort-${key}`,
        label: `${t(`sftp.${key}`)}${t(sort.key === key && sort.direction === "ascending" ? "table.sortDescending" : "table.sortAscending")}`,
        run: () => { changeSort(key); setMenu(null); },
      })),
    ];
  }

  const pathPieces = path.split("/").filter(Boolean);
  const breadcrumbPaths = pathPieces.map((_, index) => `/${pathPieces.slice(0, index + 1).join("/")}`);

  return (
    <section ref={panelRoot} className="flex h-full min-h-0 min-w-0 flex-col gap-1.5 md:gap-1" aria-labelledby={headingId}>
      <h2 id={headingId} className="sr-only">{t("sftp.heading")}</h2>
      {mobileInteraction ? (
        <div className="flex min-h-11 shrink-0 items-center gap-1 border-b border-line/50 pb-1">
          <SFTPHostPicker aliases={aliases} hosts={hosts} value={alias} disabled={dirty} onChange={selectHost} compact includeLocal />
          <button type="button" aria-label={t("sftp.back")} disabled={busy || dirty || navigation.index <= 0} onClick={() => void navigateHistory(-1)} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint">←</button>
          {pathEditing ? (
            <form className="flex min-w-0 flex-1 items-center gap-1" onSubmit={(event) => { event.preventDefault(); if (!busy && !dirty) void load(pathDraft); }}>
              <input ref={pathInput} aria-label={t("sftp.path")} value={pathDraft} onChange={(event) => setPathDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") { setPathDraft(path); setPathEditing(false); } }} className="h-11 min-w-0 w-full rounded border border-control-line bg-control px-2 font-mono text-base" />
              <Button type="submit" disabled={busy || dirty || !connected}>{t("sftp.go")}</Button>
            </form>
          ) : (
            <button type="button" data-testid="sftp-current-path" data-path={path} aria-label={t("sftp.editPath")} title={path} disabled={busy || dirty || !connected} onClick={() => setPathEditing(true)} className="flex h-11 min-w-0 flex-1 items-center gap-1 rounded px-2 text-left active:bg-select-fill disabled:text-ink-faint">
              <span className="truncate font-mono text-sm font-medium">{pathPieces.at(-1) || "/"}</span><Icon name="chevronRight" className="size-3 shrink-0 rotate-90 text-ink-muted" />
            </button>
          )}
          {pathEditing ? <button type="button" aria-label={t("sftp.cancel")} onClick={() => { setPathDraft(path); setPathEditing(false); }} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted"><Icon name="close" className="size-4" /></button> : <>
            <button type="button" aria-label={t("sftp.refreshDirectory")} disabled={busy || dirty || !connected} onClick={refreshCurrentDirectory} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint"><Icon name="sync" className="size-4" /></button>
            <button type="button" aria-label={t("sftp.mobile.search")} aria-expanded={mobileSearchOpen} disabled={busy || !connected} onClick={() => setMobileSearchOpen((value) => !value)} className={`flex size-11 shrink-0 items-center justify-center rounded active:bg-select-fill ${mobileSearchOpen || filter !== "" ? "text-accent" : "text-ink-muted"}`}><Icon name="search" className="size-4" /></button>
            <button type="button" aria-label={t("sftp.mobile.actions")} aria-haspopup="dialog" aria-expanded={menu?.kind === "folder"} onClick={(event) => toggleMenu("folder", event.currentTarget)} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="moreHorizontal" className="size-4" /></button>
          </>}
        </div>
      ) : (
      <div className="flex flex-wrap items-center gap-1.5 border-b border-line/50 pb-1.5 md:pb-1">
        <SFTPHostPicker aliases={aliases} hosts={hosts} value={alias} disabled={dirty} onChange={selectHost} includeLocal />
        {onOpenTerminal === undefined ? null : (
          <button
            type="button"
            aria-label={t("sftp.openTerminalHere")}
            title={t("sftp.openTerminalHere")}
            disabled={busy || dirty || !connected || path === ""}
            onClick={() => void onOpenTerminal(alias, path)}
            className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:size-8"
          >
            <Icon name="terminal" className="size-4" />
          </button>
        )}
        <SFTPNavigationControls busy={busy || dirty} canBack={navigation.index > 0}
          canForward={navigation.index >= 0 && navigation.index < navigation.paths.length - 1}
          canHome={connected} canRoot={connected && path !== "/"}
          onBack={() => void navigateHistory(-1)} onForward={() => void navigateHistory(1)}
          onHome={() => void load("")} onRoot={() => void load("/")} />
        <button type="button" aria-label={t("sftp.refreshDirectory")} title={t("sftp.refreshDirectory")} disabled={busy || dirty || !connected} onClick={refreshCurrentDirectory} className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:size-8"><Icon name="sync" className="size-4" /></button>
        {pathEditing ? (
          <>
            <input
              ref={pathInput}
              aria-label={t("sftp.path")}
              value={pathDraft}
              onChange={(event) => setPathDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape") {
                  event.preventDefault();
                  setPathDraft(path);
                  setPathEditing(false);
                } else if (event.key === "Enter" && !dirty) {
                  event.preventDefault();
                  void load(pathDraft);
                }
              }}
              className="min-w-44 grow rounded-md border border-control-line/70 bg-control px-2 py-1.5 font-mono text-sm outline-none focus:border-accent md:py-1"
            />
            <Button disabled={busy || dirty || !connected} onClick={() => void load(pathDraft)}>{t("sftp.go")}</Button>
          </>
        ) : (
          <div className="flex min-w-44 grow items-center rounded-md bg-control/60 px-1" data-testid="sftp-current-path" data-path={path}>
            <nav aria-label={t("sftp.path")} onClick={(event) => { if (event.target === event.currentTarget && !busy && !dirty && connected) setPathEditing(true); }}
              title={t("sftp.editPath")} className="flex min-w-0 grow cursor-text items-center overflow-x-auto whitespace-nowrap font-mono text-sm">
              <button type="button" disabled={busy || dirty || !connected || path === "/"} onClick={() => void load("/")} className="rounded px-1.5 py-1.5 text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:py-1">/</button>
              {pathPieces.map((piece, index) => (
                <span key={breadcrumbPaths[index]} className="flex min-w-0 items-center">
                  {index > 0 || path !== "/" ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
                  {index === pathPieces.length - 1 ? (
                    <span className="max-w-48 truncate px-1.5 py-1.5 font-medium text-ink md:py-1" title={piece}>{piece}</span>
                  ) : (
                    <button type="button" disabled={busy || dirty || !connected} onClick={() => void load(breadcrumbPaths[index])} className="max-w-40 truncate rounded px-1.5 py-1.5 text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:py-1" title={piece}>{piece}</button>
                  )}
                </span>
              ))}
            </nav>
            <button type="button" aria-label={t("sftp.copyPath")} title={t("sftp.copyPath")}
              disabled={!connected} onClick={() => { void copyCurrentPath(); }}
              className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">
              <Icon name="copy" className="size-3.5" />
            </button>
            <button type="button" aria-label={t("sftp.editPath")} disabled={busy || dirty || !connected} onClick={() => setPathEditing(true)} className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">
              <Icon name="edit" className="size-3.5" />
            </button>
          </div>
        )}
      </div>
      )}

      {problem === "" || listingFailed ? null : <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}
      {search === null ? null : (
        <p role="status" className="flex items-center gap-3 rounded-md border border-line bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
          <span className="min-w-0 grow truncate">
            {t(search.truncated ? "sftp.searchResultsTruncated" : "sftp.searchResults", {
              count: search.entries.length,
              query: search.query,
              path: search.root,
            })}
          </span>
          <button type="button" disabled={busy} onClick={endSearch} className="shrink-0 text-accent disabled:text-ink-faint">{t("sftp.searchEnd")}</button>
        </p>
      )}
      {undo === null ? null : (
        <p role="status" className="flex items-center gap-3 rounded-md border border-line bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
          <span className="min-w-0 grow truncate">{undo.label}</span>
          <button type="button" disabled={busy} onClick={() => void undo.run()} className="shrink-0 text-accent disabled:text-ink-faint">{t("sftp.undo")}</button>
          <button type="button" aria-label={t("sftp.dismissUndo")} onClick={() => setUndo(null)} className="flex size-6 shrink-0 items-center justify-center rounded text-ink-muted hover:text-ink">
            <Icon name="close" className="size-3" />
          </button>
        </p>
      )}

      <div className="grid min-h-0 min-w-0 flex-1 grid-cols-1 gap-2">
        <div
          aria-label={t("sftp.dropZone")}
          aria-busy={busy}
          className={`relative flex min-h-0 min-w-0 flex-col rounded-md border bg-card transition-shadow ${dragging ? "border-accent ring-1 ring-accent" : "border-line/60"}`}
          onDragEnter={(event) => { event.preventDefault(); if (!busy && connected) setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDrop={(event) => { void acceptDrop(event); }}
        >
          <div ref={menuRoot} hidden={mobileInteraction && selectedEntries.length === 0 && !mobileSearchOpen} className={mobileInteraction && selectedEntries.length === 0 && !mobileSearchOpen ? "hidden" : "relative flex min-h-10 shrink-0 items-center gap-1 border-b border-line/50 bg-toolbar/45 px-2 py-1 md:min-h-8 md:py-0.5"}>
            {selectedEntries.length > 0 && !(mobileInteraction && mobileSearchOpen) ? (
              <>
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
                <label className={mobileInteraction ? "hidden" : "relative min-w-20 max-w-32 grow"}>
                  <span className="sr-only">{t("sftp.filter")}</span>
                  <Icon name="search" className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-ink-muted" />
                  <input
                    type="search"
                    aria-label={t("sftp.filter")}
                    value={filter}
                    onChange={(event) => setFilter(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key !== "Enter") return;
                      event.preventDefault();
                      void runSearch();
                    }}
                    placeholder={t("sftp.filterPlaceholder")}
                    className="h-8 w-full rounded-md border border-control-line/60 bg-control/70 py-1 pl-7 pr-2 text-xs outline-none focus:border-accent md:h-7"
                  />
                </label>
                {compactViewport ? null : <button type="button" disabled={busy} onClick={showDetails} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.details")}</button>}
                {compactViewport ? null : <button type="button" disabled={busy || !selectedEntries.some((entry) => entry.type === "file" || entry.type === "directory")} onClick={() => void downloadEntries(selectedEntries)} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.download")}</button>}
                {compactViewport || selectedEntry === null ? null : <button type="button" disabled={busy} onClick={renameSelection} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.rename")}</button>}
                {compactViewport ? null : <button type="button" disabled={busy} onClick={deleteSelection} className="rounded px-2 py-1 text-xs text-danger hover:bg-hover disabled:text-ink-faint">{t("sftp.delete")}</button>}
                <button
                  type="button"
                  aria-label={selectionMenuLabel()}
                  aria-haspopup="menu"
                  aria-expanded={!mobileInteraction && menu?.kind === "selected"}
                  onClick={(event) => toggleMenu("selected", event.currentTarget)}
                  className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none md:size-7"
                >
                  <Icon name="moreHorizontal" className="size-4" />
                </button>
              </>
            ) : mobileInteraction ? (
              <>
                <input ref={searchInput} type="search" aria-label={t("sftp.filter")} value={filter} onChange={(event) => setFilter(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void runSearch(); event.currentTarget.blur(); } }} placeholder={t("sftp.filterPlaceholder")} className="h-11 min-w-0 flex-1 rounded-md border border-control-line bg-control px-3 text-base" />
                <button type="button" aria-label={t("sftp.searchBelow")} disabled={busy || !connected || filter.trim() === ""} onClick={() => { searchInput.current?.blur(); void runSearch(); }} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted disabled:text-ink-faint"><Icon name="search" className="size-4" /></button>
                <button type="button" aria-label={t("sftp.close")} onClick={() => { setMobileSearchOpen(false); setFilter(""); }} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted"><Icon name="close" className="size-4" /></button>
              </>
            ) : (
            <>
            <button
              type="button"
              aria-label={t("sftp.createActions")}
              aria-haspopup="menu"
              aria-expanded={!mobileInteraction && menu?.kind === "create"}
              disabled={busy || !connected}
              onClick={(event) => toggleMenu("create", event.currentTarget)}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
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
                onKeyDown={(event) => {
                  if (event.key !== "Enter") return;
                  event.preventDefault();
                  void runSearch();
                }}
                placeholder={t("sftp.filterPlaceholder")}
                className="h-8 w-full rounded-md border border-control-line/60 bg-control/70 py-1 pl-7 pr-2 text-xs outline-none focus:border-accent md:h-7"
              />
            </label>
            <button
              type="button"
              aria-label={t("sftp.searchBelow")}
              title={t("sftp.searchBelow")}
              disabled={busy || !connected || filter.trim() === ""}
              onClick={() => void runSearch()}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
            >
              <Icon name="search" className="size-4" />
            </button>
            <span className="hidden min-w-0 grow truncate text-xs text-ink-muted lg:block">{t(dragging ? "sftp.dropNow" : "sftp.dropHint")}</span>
            </>
            )}
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
            {!mobileInteraction && menu?.kind === "create" ? (
              <div ref={menuPanel} role="menu" aria-label={t("sftp.createActions")} className="absolute left-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "mkdir" }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFolder")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "createFile" }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFile")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); upload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.upload")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); folderUpload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.uploadFolder")}</button>
              </div>
            ) : null}
            {!mobileInteraction && menu?.kind === "selected" && selectedEntries.length > 0 ? (
              <div ref={menuPanel} role="menu" aria-label={selectionMenuLabel()} className="absolute right-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <MenuActionList actions={selectedMenuActions()} />
              </div>
            ) : null}
            {!mobileInteraction && menu?.kind === "context" && selectedEntries.length > 0 ? (
              <div
                ref={menuPanel}
                role="menu"
                aria-label={selectionMenuLabel()}
                style={{
                  left: Math.max(8, Math.min(menu.x, window.innerWidth - contextMenuWidth - 8)),
                  top: Math.max(8, Math.min(menu.y, window.innerHeight - selectedMenuActions().length * contextMenuItemHeight - 16)),
                }}
                className="fixed z-30 w-56 rounded-lg border border-control-line bg-card p-1 shadow-lg"
              >
                <MenuActionList actions={selectedMenuActions()} />
              </div>
            ) : null}
          </div>
          {connected && pendingPath !== null ? <div role="status" className="absolute inset-0 z-10 flex items-center justify-center rounded-md bg-card/80 p-4 backdrop-blur-[1px]"><span className="flex min-w-0 items-center gap-3 rounded-lg bg-toolbar px-4 py-3 text-sm"><span aria-hidden="true" className="size-4 shrink-0 animate-spin rounded-full border-2 border-accent border-t-transparent motion-reduce:animate-none" /><span className="min-w-0"><span className="block">{t("sftp.loading")}</span><span className="block truncate font-mono text-xs text-ink-muted">{pendingPath || t("sftp.homeDirectory")}</span></span></span></div> : null}
          <div data-testid="sftp-file-list" className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain" inert={connected && busy} onKeyDown={list.handleListKeys}>
            {alias === "" ? (
              <PanelState tone="empty" title={t("sftp.chooseHost")} detail={t("sftp.chooseHostHint")} />
            ) : !connected && busy ? (
              <PanelState tone="loading" title={t("sftp.connecting", { alias })} />
            ) : !connected && problem !== "" ? (
              <PanelState
                tone="failed"
                title={problem}
                action={<Button onClick={() => void load(path, alias)}>{t("sftp.retry")}</Button>}
              />
            ) : !connected ? (
              <PanelState
                tone="empty"
                title={t("sftp.readyToConnect", { alias })}
                detail={t("sftp.connectHint")}
                action={<Button kind="primary" onClick={() => void load(path, alias)}>{t("sftp.connect")}</Button>}
              />
            ) : busy && entries.length === 0 && problem === "" ? (
              <PanelState tone="loading" title={t("sftp.loading")} />
            ) : listingFailed ? (
              <PanelState
                tone="failed"
                title={problem}
                action={<Button onClick={() => void load(path)}>{t("sftp.retry")}</Button>}
              />
            ) : search !== null && displayedEntries.length === 0 ? (
              <PanelState
                tone="empty"
                title={t("sftp.searchNoMatches", { query: search.query })}
                action={<Button onClick={endSearch}>{t("sftp.searchEnd")}</Button>}
              />
            ) : displayedEntries.length === 0 ? (
              <PanelState
                tone="empty"
                title={t(normalizedFilter === "" ? "sftp.emptyDirectory" : "sftp.noFilterMatches")}
                {...(normalizedFilter === "" ? {} : { detail: t("sftp.clearFilterHint") })}
                action={parentRowVisible ? (
                  <Button disabled={busy || dirty} onClick={openParent}>
                    {t("sftp.parentDirectory")}
                  </Button>
                ) : undefined}
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
                locked={dirty}
                parentRowVisible={parentRowVisible}
                draggable={(entry) => entry.type === "file" || entry.type === "directory"}
                onDragStart={beginRemoteDrag}
                entryContext={search === null ? undefined : (entry) => parentOf(entry.path)}
              />
            )}
          </div>
          {showTransfers ? <TransferManagerList openRequest={openQueueRequest} /> : null}
        </div>

      </div>

      {mobileInteraction && menu !== null ? (
        <ModalShell labelledBy={`${headingId}-mobile-actions`} onDismiss={() => setMenu(null)} closeOnOutside returnFocusRef={menuTrigger} placement="sheet" panelClassName="flex max-h-[80dvh] w-full max-w-lg flex-col overflow-hidden rounded-xl">
          <div className="flex shrink-0 items-center justify-between gap-2 border-b border-line px-4 py-1">
            <h3 id={`${headingId}-mobile-actions`} className="min-w-0 truncate font-medium">{menu.kind === "selected" || menu.kind === "context" ? selectionMenuLabel() : t("sftp.mobile.actions")}</h3>
            <button type="button" aria-label={t("sftp.close")} onClick={() => setMenu(null)} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted"><Icon name="close" className="size-4" /></button>
          </div>
          <div role="menu" className="min-h-0 overflow-y-auto overscroll-contain p-2">
            {menu.kind === "selected" || menu.kind === "context" ? <MenuActionList actions={selectedMenuActions()} /> : <MenuActionList actions={folderMenuActions()} />}
          </div>
        </ModalShell>
      ) : null}

      {remoteDrop === null ? null : (
        <ModalShell labelledBy={`${headingId}-remote-drop`} onDismiss={() => setRemoteDrop(null)} panelClassName="w-full max-w-md rounded-lg p-5">
          <h3 id={`${headingId}-remote-drop`} className="text-base font-semibold text-ink">{t("sftp.remoteDropTitle")}</h3>
          <p className="mt-2 text-sm leading-6 text-ink-muted">
            {t("sftp.remoteDropDescription", { count: remoteDrop.entries.length, alias })}
          </p>
          <div className="mt-5 flex flex-wrap justify-end gap-2">
            <Button onClick={() => setRemoteDrop(null)}>{t("sftp.cancel")}</Button>
            <Button onClick={() => void acceptRemoteDrop("move")}>{t("sftp.moveHere")}</Button>
            <Button kind="primary" onClick={() => void acceptRemoteDrop("copy")}>{t("sftp.copyHere")}</Button>
          </div>
        </ModalShell>
      )}

      <SFTPTextEditor editor={editor} busy={browsing} />

      {details === null ? null : (
        <SFTPDetailsDialog
          alias={alias}
          entries={details}
          busy={busy}
          returnFocusRef={activeRow}
          onClose={() => setDetails(null)}
          onEdit={(entry) => { setDetails(null); void editor.open(alias, entry); }}
          onDownload={(targets) => { setDetails(null); void downloadEntries(targets); }}
          onRename={(entry) => { setDetails(null); setInputIntent({ kind: "rename", entry }); }}
        />
      )}

      {deleting === null ? null : (
        <ConfirmDialog
          id={`${headingId}-delete`}
          heading={deleting.length === 1 ? t("sftp.deleteHeading") : t("sftp.deleteHeadingCount", { count: deleting.length })}
          body={<div className="space-y-2 text-sm text-ink-muted">
            {deleting.some((entry) => entry.type === "directory") ? <p>{t("sftp.deleteContentsWarning")}</p> : null}
            <ul className="max-h-48 space-y-1 overflow-auto">
              {deleting.map((entry) => <li key={entry.path} className="break-all font-mono">{entry.path}</li>)}
            </ul>
          </div>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={activeRow}
          onConfirm={() => void remove()}
          onCancel={() => { pendingFocus.current = null; setDeleting(null); }}
        />
      )}
      {inputIntent === null ? null : (
        <InputDialog
          id={`${headingId}-input`}
          heading={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "createFile" ? "sftp.newFile" : inputIntent.kind === "rename" ? "sftp.rename" : inputIntent.kind === "duplicate" ? "sftp.duplicate" : inputIntent.kind === "moveTo" ? "sftp.moveTo" : inputIntent.recursive ? "sftp.chmodRecursive" : "sftp.chmod")}
          label={t(inputIntent.kind === "chmod" ? "sftp.chmodPrompt" : inputIntent.kind === "rename" || inputIntent.kind === "duplicate" ? "sftp.renamePrompt" : inputIntent.kind === "createFile" ? "sftp.newFilePrompt" : inputIntent.kind === "moveTo" ? "sftp.moveToPrompt" : "sftp.mkdirPrompt")}
          initialValue={inputIntent.kind === "mkdir" || inputIntent.kind === "createFile" ? "" : inputIntent.kind === "rename" ? inputIntent.entry.name : inputIntent.kind === "duplicate" ? `${inputIntent.entry.name}.copy` : inputIntent.kind === "moveTo" ? path : symbolicModeToOctal(inputIntent.entry.mode)}
          inputMode={inputIntent.kind === "chmod" ? "numeric" : "text"}
          submitLabel={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "createFile" ? "sftp.newFile" : inputIntent.kind === "rename" ? "sftp.rename" : inputIntent.kind === "duplicate" ? "sftp.duplicate" : inputIntent.kind === "moveTo" ? "sftp.move" : "sftp.chmod")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={activeRow}
          validate={(value) => {
            if (inputIntent.kind === "chmod") return /^0?[0-7]{3}$/.test(value) ? "" : t("sftp.chmodInvalid");
            if (inputIntent.kind === "moveTo") return value.startsWith("/") ? "" : t("sftp.pathAbsolute");
            if (value === "") return t("sftp.nameRequired");
            if (value.includes("/")) return t("sftp.nameInvalid");
            if (inputIntent.kind === "rename" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            if (inputIntent.kind === "duplicate" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            return "";
          }}
          onSubmit={(value) => {
            const intent = inputIntent;
            setInputIntent(null);
            if (intent.kind === "mkdir") void makeDirectory(value);
            else if (intent.kind === "createFile") void makeEmptyFile(value);
            else if (intent.kind === "rename") void rename(intent.entry, value);
            else if (intent.kind === "duplicate") void queueRemoteOperation([intent.entry], "copy", () => join(parentOf(intent.entry.path), value));
            else if (intent.kind === "moveTo") void queueRemoteOperation(intent.entries, "move", (entry) => join(value, entry.name));
            else void chmod(intent.entry, value, intent.recursive);
          }}
          onCancel={() => setInputIntent(null)}
        />
      )}
    </section>
  );
}

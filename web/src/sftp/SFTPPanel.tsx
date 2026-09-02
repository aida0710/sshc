import {
  Suspense,
  lazy,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type DragEvent as ReactDragEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { failureCode } from "../api/client";
import type { HostEntry } from "../api/config";
import type { BrowserLocation, NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { clipboard } from "../ui/clipboard";
import { InputDialog } from "../ui/InputDialog";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
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
import { formatBytes } from "./format";
import { sftpPlaces } from "./places";
import { SFTPDetailsDialog } from "./SFTPDetailsDialog";
import { directoryPaths, safeRelativePath, symbolicModeToOctal, type LocalTransferFile } from "./transfers";
import { TransferManagerList } from "./TransferManagerList";
import { sftpTransferManager } from "./transferManager";
import { SFTPHostPicker } from "./SFTPHostPicker";

const MonacoEditor = lazy(() =>
  import("./MonacoEditor").then(({ MonacoEditor }) => ({ default: MonacoEditor })),
);
const noHosts: HostEntry[] = [];
const remoteEntriesMime = "application/x-sshc-sftp-entries";

type RemoteDragPayload = {
  alias: string;
  entries: Array<Pick<RemoteEntry, "name" | "path" | "type" | "size">>;
};

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

// The overflow button and the row context menu open the same list of actions.
// Anchoring them to one shape keeps a right click from offering less than the
// three-dot button placed above the same rows.
type SFTPMenu =
  | { kind: "create" }
  | { kind: "places" }
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
  | { kind: "rename"; entry: RemoteEntry }
  | { kind: "chmod"; entry: RemoteEntry };

// The parent row keeps its own key so that arrow navigation can land on it
// without pretending that ".." is a listed entry.
const parentRowKey = "..";

const contextMenuWidth = 224;
const contextMenuItemHeight = 40;
const longPressDelay = 500;
const longPressSlack = 12;

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
          className={`block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0 ${action.danger === true ? "text-danger" : ""}`}
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
  showTransfers = true,
  onTargetHandled = () => undefined,
  onLocationChange = () => undefined,
  onNavigationBlockerChange,
  onNavigateLocation,
  onOpenTerminal,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  // Where a restored tab should reopen. Applied once, when the declared
  // aliases have arrived and can vouch for the host.
  initialLocation?: { alias: string; path: string } | null;
  showTransfers?: boolean;
  onTargetHandled?: (request: number) => void;
  onLocationChange?: (alias: string, path: string) => void;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string) => void) | undefined;
  onOpenTerminal?: ((alias: string, path: string) => void | Promise<void>) | undefined;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [path, setPath] = useState("");
  const [pathDraft, setPathDraft] = useState("");
  const [pathEditing, setPathEditing] = useState(false);
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [opened, setOpened] = useState<RemoteTextFile | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [deleting, setDeleting] = useState<RemoteEntry[] | null>(null);
  const [details, setDetails] = useState<RemoteEntry[] | null>(null);
  // While a search is showing, the list is its results rather than one
  // directory. Everything downstream reads listedEntries, not entries.
  const [search, setSearch] = useState<{ root: string; query: string; entries: RemoteEntry[]; truncated: boolean } | null>(null);
  const [leaving, setLeaving] = useState<BrowserLocation | null>(null);
  // What the last change was, and how to put it back. Delete is absent on
  // purpose: SFTP has no trash, so an "undo" there would be a lie.
  const [undo, setUndo] = useState<{ label: string; run: () => Promise<void> } | null>(null);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(() => new Set());
  const [navigation, setNavigation] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const [filter, setFilter] = useState("");
  const [menu, setMenu] = useState<SFTPMenu | null>(null);
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const [remoteDrop, setRemoteDrop] = useState<RemoteDragPayload | null>(null);
  const [compactViewport, setCompactViewport] = useState(compactSFTPViewport);
  const [sort, setSort] = useState<{ key: SFTPSort; direction: SortDirection }>({
    key: "name",
    direction: "ascending",
  });
  const upload = useRef<HTMLInputElement>(null);
  const folderUpload = useRef<HTMLInputElement>(null);
  const pathInput = useRef<HTMLInputElement>(null);
  const panelRoot = useRef<HTMLElement>(null);
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
  const selectAll = useRef<HTMLInputElement>(null);
  const selectionAnchor = useRef<string | null>(null);
  const rowNodes = useRef(new Map<string, HTMLElement>());
  // Dialogs opened from a menu outlive their trigger, so they are handed the
  // row itself as the element that takes focus back.
  const activeRow = useRef<HTMLElement | null>(null);
  const pendingFocus = useRef<string | null>(null);
  const longPress = useRef<{ timer: ReturnType<typeof globalThis.setTimeout>; x: number; y: number } | null>(null);
  const suppressNextClick = useRef(false);

  useDismissibleLayer({
    open: menu !== null,
    containerRefs: [menuRoot],
    onDismiss: () => setMenu(null),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menu !== null, menuRef: menuPanel, onClose: () => setMenu(null) });

  useEffect(() => {
    const element = panelRoot.current;
    const media = typeof window.matchMedia === "function" ? window.matchMedia("(max-width: 767px)") : null;
    const update = () => {
      const width = element?.clientWidth ?? 0;
      setCompactViewport((media?.matches ?? false) || (width > 0 && width < 680));
    };
    update();
    media?.addEventListener("change", update);
    const observer = typeof ResizeObserver === "undefined" || element === null ? null : new ResizeObserver(update);
    if (element !== null) observer?.observe(element);
    return () => {
      media?.removeEventListener("change", update);
      observer?.disconnect();
    };
  }, []);
  useEffect(() => {
    if (!pathEditing) return;
    pathInput.current?.focus();
    pathInput.current?.select();
  }, [pathEditing]);
  useEffect(() => () => {
    if (longPress.current !== null) globalThis.clearTimeout(longPress.current.timer);
  }, []);

  const transferJobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  useSyncExternalStore(sftpPlaces.subscribe, sftpPlaces.getSnapshot);
  const refreshedUploads = useRef(new Set<string>());
  const dirty = opened !== null && contents !== opened.contents;
  const listedEntries = search === null ? entries : search.entries;
  const sortedEntries = ordered(
    listedEntries,
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
  // The filter box is the query in search mode; matching again locally would
  // hide results whose match is in a parent directory's name.
  const displayedEntries = normalizedFilter === "" || search !== null
    ? sortedEntries
    : sortedEntries.filter((entry) => entry.name.toLocaleLowerCase().includes(normalizedFilter));
  const selectedEntries = listedEntries.filter((entry) => selectedPaths.has(entry.path));
  const selectedEntry = selectedEntries.length === 1 ? selectedEntries[0] ?? null : null;
  const allDisplayedSelected = displayedEntries.length > 0 && displayedEntries.every((entry) => selectedPaths.has(entry.path));
  const parentRowVisible = search === null && path !== "" && path !== "/";
  const rowKeys = [...(parentRowVisible ? [parentRowKey] : []), ...displayedEntries.map((entry) => entry.path)];
  // Exactly one row owns the tab stop. A filter or a reload can drop the
  // remembered row, so fall back to the first one instead of stranding the
  // keyboard outside the list.
  const activeRowKey = focusedKey !== null && rowKeys.includes(focusedKey) ? focusedKey : rowKeys[0] ?? null;
  const bookmarkedPaths = sftpPlaces.bookmarks(alias);
  // A bookmarked path is already one click away; repeating it under "recent"
  // only makes the menu longer and the two lists ambiguous.
  const recentPaths = sftpPlaces.recent(alias)
    .filter((candidate) => candidate !== path && !bookmarkedPaths.includes(candidate));
  const bookmarkedHere = path !== "" && sftpPlaces.bookmarked(alias, path);
  // A listing that failed says so where the rows would be, with the retry next
  // to it. Repeating the same sentence in the banner above would be two voices
  // for one fact.
  const listingFailed = problem !== "" && alias !== "" && entries.length === 0;

  useEffect(() => {
    if (!dirty) {
      onNavigationBlockerChange?.(null);
      return;
    }
    onNavigationBlockerChange?.((next) => {
      setLeaving(next);
      return false;
    });
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => {
      onNavigationBlockerChange?.(null);
      window.removeEventListener("beforeunload", warnBeforeUnload);
    };
  }, [dirty, onNavigationBlockerChange]);

  useEffect(() => {
    if (selectAll.current !== null) {
      selectAll.current.indeterminate = selectedEntries.length > 0 && !allDisplayedSelected;
    }
  }, [allDisplayedSelected, selectedEntries.length]);

  useEffect(() => {
    activeRow.current = activeRowKey === null ? null : rowNodes.current.get(activeRowKey) ?? null;
  });

  useEffect(() => {
    const key = pendingFocus.current;
    if (key === null) return;
    pendingFocus.current = null;
    const node = rowNodes.current.get(key);
    if (node === undefined) return;
    setFocusedKey(key);
    node.focus();
  }, [entries]);

  function changeSort(key: SFTPSort) {
    setSort((current) => nextSort(current.key, current.direction, key));
  }

  function selectHost(nextAlias: string) {
    // Invalidate every request started for the previous host before React runs
    // the alias effect. Keeping its rows visible would also let an action for
    // host A be submitted with host B's alias during the hand-off render.
    loadGeneration.current += 1;
    reportLocation.current(nextAlias, "");
    setAlias(nextAlias);
    setPath("");
    setPathDraft("");
    setEntries([]);
    setOpened(null);
    setContents("");
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
      setPathEditing(false);
      setEntries(listing.entries);
      sftpPlaces.remember(nextAlias, listing.path);
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
    if (initialLocation === null || openedInitialLocation.current) return;
    if (!aliases.includes(initialLocation.alias) || !initialLocation.path.startsWith("/")) return;
    openedInitialLocation.current = true;
    openingTarget.current = true;
    selectHost(initialLocation.alias);
    void load(initialLocation.path, initialLocation.alias).finally(() => { openingTarget.current = false; });
    // A restored tab reopens once. The ref, not the dependency list, is what
    // keeps a re-render from reloading the directory under the user.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aliases, initialLocation?.alias, initialLocation?.path]);

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
      pendingFocus.current = join(targetPath, name);
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
    await refreshAfterChange(targetPath, targetAlias);
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
    const previous = symbolicModeToOctal(entry.mode);
    setBusy(true);
    try {
      await sftpApi.chmod(targetAlias, entry.path, mode, entry.revision);
      if (generation !== loadGeneration.current) return;
      const reloaded = await load(targetPath, targetAlias, true);
      const current = reloaded?.find((candidate) => candidate.path === entry.path);
      if (current !== undefined && previous !== mode) {
        offerUndo(t("sftp.permissionsChanged", { mode }), async () => {
          await sftpApi.chmod(targetAlias, entry.path, previous, current.revision);
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
      if (generation === loadGeneration.current) setBusy(false);
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

  function activate(entry: RemoteEntry) {
    setMenu(null);
    if (entry.type === "directory") void load(entry.path);
    else setDetails([entry]);
  }

  function showDetails() {
    if (selectedEntries.length === 0) return;
    setMenu(null);
    setDetails(selectedEntries);
  }

  function selectEntry(entry: RemoteEntry, modifiers: { shift?: boolean; additive?: boolean } = {}) {
    const anchorIndex = selectionAnchor.current === null
      ? -1
      : displayedEntries.findIndex((candidate) => candidate.path === selectionAnchor.current);
    const entryIndex = displayedEntries.findIndex((candidate) => candidate.path === entry.path);
    if (modifiers.shift === true && anchorIndex >= 0 && entryIndex >= 0) {
      const start = Math.min(anchorIndex, entryIndex);
      const end = Math.max(anchorIndex, entryIndex);
      setSelectedPaths((current) => {
        const next = modifiers.additive === true ? new Set(current) : new Set<string>();
        for (const candidate of displayedEntries.slice(start, end + 1)) next.add(candidate.path);
        return next;
      });
    } else if (modifiers.additive === true) {
      toggleSelection(entry);
      selectionAnchor.current = entry.path;
      return;
    } else {
      setSelectedPaths(new Set([entry.path]));
      selectionAnchor.current = entry.path;
    }
    setMenu(null);
  }

  function clickEntry(entry: RemoteEntry, event: ReactMouseEvent<HTMLButtonElement>) {
    // A long press already opened the context menu for this row. The synthetic
    // click that follows the touch must not close it again.
    if (suppressNextClick.current) {
      suppressNextClick.current = false;
      return;
    }
    setFocusedKey(entry.path);
    selectEntry(entry, { shift: event.shiftKey, additive: event.metaKey || event.ctrlKey });
  }

  function focusRow(key: string) {
    setFocusedKey(key);
    const node = rowNodes.current.get(key);
    node?.focus();
    node?.scrollIntoView?.({ block: "nearest" });
  }

  function registerRow(key: string, node: HTMLElement | null): void {
    if (node === null) rowNodes.current.delete(key);
    else rowNodes.current.set(key, node);
  }

  // The row that owns the keystroke is read from the DOM rather than from
  // state, so that tabbing or clicking into a row is honoured even before the
  // focus event has been reduced into React state.
  function currentRowKey(target: EventTarget | null): string | null {
    const element = target instanceof Element ? target.closest("[data-row-key]") : null;
    return element?.getAttribute("data-row-key") ?? activeRowKey;
  }

  function moveRowFocus(event: ReactKeyboardEvent<HTMLDivElement>, from: string | null) {
    if (rowKeys.length === 0) return;
    const current = from === null ? -1 : rowKeys.indexOf(from);
    const last = rowKeys.length - 1;
    const next = event.key === "Home"
      ? 0
      : event.key === "End"
        ? last
        : event.key === "ArrowDown"
          ? current < 0 ? 0 : Math.min(last, current + 1)
          : current < 0 ? last : Math.max(0, current - 1);
    const destination = rowKeys[next];
    if (destination === undefined) return;
    focusRow(destination);
    const entry = displayedEntries.find((candidate) => candidate.path === destination);
    // Ctrl moves the cursor without disturbing a multi-row selection, and the
    // parent row is a destination rather than something selectable.
    if (entry === undefined || event.ctrlKey || event.metaKey) return;
    selectEntry(entry, { shift: event.shiftKey, additive: false });
  }

  function entryForKey(key: string | null): RemoteEntry | null {
    return displayedEntries.find((candidate) => candidate.path === key) ?? null;
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

  function handleListKeys(event: ReactKeyboardEvent<HTMLDivElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "a") {
      event.preventDefault();
      selectAllDisplayed();
      return;
    }
    // A checkbox owns Space, and the browser owns typing inside inputs.
    const withinInput = event.target instanceof HTMLInputElement;
    const rowKey = currentRowKey(event.target);
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp":
      case "Home":
      case "End":
        event.preventDefault();
        moveRowFocus(event, rowKey);
        return;
      case " ": {
        if (withinInput) return;
        const entry = entryForKey(rowKey);
        if (entry === null) return;
        event.preventDefault();
        toggleSelection(entry);
        return;
      }
      case "Enter": {
        if (withinInput || busy || dirty) return;
        if (rowKey === parentRowKey) {
          event.preventDefault();
          pendingFocus.current = parentRowKey;
          void load(parentOf(path));
          return;
        }
        const entry = entryForKey(rowKey);
        if (entry === null) return;
        event.preventDefault();
        activate(entry);
        return;
      }
      case "F2":
        event.preventDefault();
        renameSelection();
        return;
      case "Delete":
        event.preventDefault();
        deleteSelection();
        return;
      case "Escape":
        if (selectedPaths.size > 0) {
          event.preventDefault();
          setSelectedPaths(new Set());
          selectionAnchor.current = null;
          return;
        }
        if (search === null) return;
        event.preventDefault();
        endSearch();
        return;
      default:
    }
  }

  // A right click on a row outside the selection acts on that row alone, the
  // way every file manager does; inside it, the whole selection is kept.
  function openContextMenu(entry: RemoteEntry, x: number, y: number) {
    if (!selectedPaths.has(entry.path)) {
      setSelectedPaths(new Set([entry.path]));
      selectionAnchor.current = entry.path;
    }
    focusRow(entry.path);
    menuTrigger.current = null;
    setMenu({ kind: "context", x, y });
  }

  function rowContextMenu(event: ReactMouseEvent<HTMLElement>, entry: RemoteEntry) {
    event.preventDefault();
    openContextMenu(entry, event.clientX, event.clientY);
  }

  function cancelLongPress() {
    if (longPress.current === null) return;
    globalThis.clearTimeout(longPress.current.timer);
    longPress.current = null;
  }

  function beginLongPress(event: ReactPointerEvent<HTMLElement>, entry: RemoteEntry) {
    if (event.pointerType === "mouse") return;
    cancelLongPress();
    // A long press that opened a menu but was never followed by a click must
    // not swallow the first tap on some other row.
    suppressNextClick.current = false;
    const { clientX, clientY } = event;
    const timer = globalThis.setTimeout(() => {
      longPress.current = null;
      suppressNextClick.current = true;
      openContextMenu(entry, clientX, clientY);
    }, longPressDelay);
    longPress.current = { timer, x: clientX, y: clientY };
  }

  function trackLongPress(event: ReactPointerEvent<HTMLElement>) {
    const pending = longPress.current;
    if (pending === null) return;
    if (Math.abs(event.clientX - pending.x) > longPressSlack || Math.abs(event.clientY - pending.y) > longPressSlack) {
      cancelLongPress();
    }
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

  function goTo(remotePath: string) {
    setMenu(null);
    void load(remotePath);
  }

  function toggleMenu(kind: "create" | "places" | "selected", trigger: HTMLButtonElement) {
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
        run: () => { setMenu(null); void openText(selectedEntry); },
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
        run: () => { setMenu(null); setInputIntent({ kind: "chmod", entry: selectedEntry }); },
      });
    }
    if (selectedEntry !== null) {
      actions.push({ key: "rename", label: t("sftp.rename"), disabled: busy, run: renameSelection });
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

  const pathPieces = path.split("/").filter(Boolean);
  const breadcrumbPaths = pathPieces.map((_, index) => `/${pathPieces.slice(0, index + 1).join("/")}`);

  return (
    <section ref={panelRoot} className="flex h-full min-h-0 min-w-0 flex-col gap-1.5 md:gap-1" aria-labelledby={headingId}>
      <h2 id={headingId} className="sr-only">{t("sftp.heading")}</h2>
      <div className="flex flex-wrap items-center gap-1.5 border-b border-line/50 pb-1.5 md:pb-1">
        <SFTPHostPicker aliases={aliases} hosts={hosts} value={alias} disabled={dirty} onChange={selectHost} />
        {onOpenTerminal === undefined ? null : (
          <button
            type="button"
            aria-label={t("sftp.openTerminalHere")}
            title={t("sftp.openTerminalHere")}
            disabled={busy || dirty || alias === "" || path === ""}
            onClick={() => void onOpenTerminal(alias, path)}
            className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint md:size-8"
          >
            <Icon name="terminal" className="size-4" />
          </button>
        )}
        <div role="group" aria-label={t("sftp.navigation")} className="flex shrink-0 overflow-hidden rounded-md bg-toolbar/70">
          <button type="button" aria-label={t("sftp.back")} disabled={busy || dirty || navigation.index <= 0} onClick={() => void navigateHistory(-1)} className="flex size-9 items-center justify-center text-ink-muted hover:bg-select-fill disabled:text-ink-faint md:size-8">
            <span aria-hidden="true">←</span>
          </button>
          <button type="button" aria-label={t("sftp.forward")} disabled={busy || dirty || navigation.index < 0 || navigation.index >= navigation.paths.length - 1} onClick={() => void navigateHistory(1)} className="flex size-9 items-center justify-center text-ink-muted hover:bg-select-fill disabled:text-ink-faint md:size-8">
            <span aria-hidden="true">→</span>
          </button>
          <button type="button" aria-label={t("sftp.homeDirectory")} disabled={busy || dirty || alias === ""} onClick={() => void load("")} className="flex size-9 items-center justify-center text-ink-muted hover:bg-select-fill disabled:text-ink-faint md:size-8">
            <Icon name="home" className="size-4" />
          </button>
          <button type="button" aria-label={t("sftp.rootDirectory")} disabled={busy || dirty || alias === "" || path === "/"} onClick={() => void load("/")} className="flex size-9 items-center justify-center font-mono text-sm text-ink-muted hover:bg-select-fill disabled:text-ink-faint md:size-8">
            /
          </button>
        </div>
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
            <Button disabled={busy || dirty || alias === ""} onClick={() => void load(pathDraft)}>{t("sftp.go")}</Button>
          </>
        ) : (
          <div className="flex min-w-44 grow items-center rounded-md bg-control/60 px-1" data-testid="sftp-current-path" data-path={path}>
            <nav aria-label={t("sftp.path")} className="flex min-w-0 grow items-center overflow-x-auto whitespace-nowrap font-mono text-sm">
              <button type="button" disabled={busy || dirty || alias === "" || path === "/"} onClick={() => void load("/")} className="rounded px-1.5 py-1.5 text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint md:py-1">/</button>
              {pathPieces.map((piece, index) => (
                <span key={breadcrumbPaths[index]} className="flex min-w-0 items-center">
                  {index > 0 || path !== "/" ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
                  {index === pathPieces.length - 1 ? (
                    <span className="max-w-48 truncate px-1.5 py-1.5 font-medium text-ink md:py-1" title={piece}>{piece}</span>
                  ) : (
                    <button type="button" disabled={busy || dirty} onClick={() => void load(breadcrumbPaths[index])} className="max-w-40 truncate rounded px-1.5 py-1.5 text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint md:py-1" title={piece}>{piece}</button>
                  )}
                </span>
              ))}
            </nav>
            <button type="button" aria-label={t("sftp.editPath")} disabled={busy || dirty || alias === ""} onClick={() => setPathEditing(true)} className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint">
              <Icon name="edit" className="size-3.5" />
            </button>
          </div>
        )}
      </div>

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
          className={`flex min-h-0 min-w-0 flex-col rounded-md border bg-card transition-shadow ${dragging ? "border-accent ring-1 ring-accent" : "border-line/60"}`}
          onDragEnter={(event) => { event.preventDefault(); if (!busy && alias !== "") setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDrop={(event) => { void acceptDrop(event); }}
        >
          <div ref={menuRoot} className="relative flex min-h-10 items-center gap-1 border-b border-line/50 bg-toolbar/45 px-2 py-1 md:min-h-8 md:py-0.5">
            {selectedEntries.length > 0 ? (
              <>
                <button type="button" aria-label={t("sftp.clearSelection")} onClick={() => setSelectedPaths(new Set())} className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill hover:text-ink md:size-7">
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
                <label className="relative min-w-20 max-w-32 grow">
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
                {compactViewport ? null : <button type="button" disabled={busy} onClick={showDetails} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint">{t("sftp.details")}</button>}
                {compactViewport ? null : <button type="button" disabled={busy || !selectedEntries.some((entry) => entry.type === "file" || entry.type === "directory")} onClick={() => void downloadEntries(selectedEntries)} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint">{t("sftp.download")}</button>}
                {compactViewport || selectedEntry === null ? null : <button type="button" disabled={busy} onClick={renameSelection} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-select-fill hover:text-ink disabled:text-ink-faint">{t("sftp.rename")}</button>}
                {compactViewport ? null : <button type="button" disabled={busy} onClick={deleteSelection} className="rounded px-2 py-1 text-xs text-danger hover:bg-select-fill disabled:text-ink-faint">{t("sftp.delete")}</button>}
                <button
                  type="button"
                  aria-label={selectionMenuLabel()}
                  aria-haspopup="menu"
                  aria-expanded={menu?.kind === "selected"}
                  onClick={(event) => toggleMenu("selected", event.currentTarget)}
                  className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus:bg-select-fill focus:outline-none md:size-7"
                >
                  <Icon name="moreHorizontal" className="size-4" />
                </button>
              </>
            ) : (
            <>
            <button
              type="button"
              aria-label={t("sftp.createActions")}
              aria-haspopup="menu"
              aria-expanded={menu?.kind === "create"}
              disabled={busy || alias === ""}
              onClick={(event) => toggleMenu("create", event.currentTarget)}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
            >
              <Icon name="plus" className="size-4" />
            </button>
            <button
              type="button"
              aria-label={t("sftp.places")}
              aria-haspopup="menu"
              aria-expanded={menu?.kind === "places"}
              disabled={busy || alias === ""}
              onClick={(event) => toggleMenu("places", event.currentTarget)}
              className={`flex size-10 shrink-0 items-center justify-center rounded hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7 ${bookmarkedHere ? "text-accent" : "text-ink-muted"}`}
            >
              <Icon name="star" className="size-4" />
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
              disabled={busy || alias === "" || filter.trim() === ""}
              onClick={() => void runSearch()}
              className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
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
            {menu?.kind === "create" ? (
              <div ref={menuPanel} role="menu" aria-label={t("sftp.createActions")} className="absolute left-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); setInputIntent({ kind: "mkdir" }); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFolder")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); upload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.upload")}</button>
                <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); folderUpload.current?.click(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.uploadFolder")}</button>
              </div>
            ) : null}
            {menu?.kind === "places" ? (
              <div ref={menuPanel} role="menu" aria-label={t("sftp.places")} className="absolute left-2 top-full z-20 mt-1 max-h-80 w-72 overflow-auto rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <button
                  type="button"
                  role="menuitem"
                  disabled={path === ""}
                  onClick={() => sftpPlaces.toggleBookmark(alias, path)}
                  className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0"
                >
                  {t(bookmarkedHere ? "sftp.removeBookmark" : "sftp.addBookmark")}
                </button>
                {bookmarkedPaths.length === 0 ? null : (
                  <>
                    <p className="px-2.5 pt-2 text-[11px] uppercase tracking-wide text-ink-faint">{t("sftp.bookmarks")}</p>
                    {bookmarkedPaths.map((bookmark) => (
                      <span key={bookmark} className="flex items-center gap-1">
                        <button type="button" role="menuitem" onClick={() => goTo(bookmark)} className="block min-h-10 min-w-0 grow truncate rounded px-2.5 py-2 text-left font-mono text-xs hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{bookmark}</button>
                        <button type="button" role="menuitem" aria-label={t("sftp.removeBookmarkFor", { path: bookmark })} onClick={() => sftpPlaces.removeBookmark(alias, bookmark)} className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill hover:text-danger focus:bg-select-fill focus:outline-none">
                          <Icon name="close" className="size-3" />
                        </button>
                      </span>
                    ))}
                  </>
                )}
                {recentPaths.length === 0 ? null : (
                  <>
                    <p className="px-2.5 pt-2 text-[11px] uppercase tracking-wide text-ink-faint">{t("sftp.recentPaths")}</p>
                    {recentPaths.map((recent) => (
                      <button key={recent} type="button" role="menuitem" onClick={() => goTo(recent)} className="block min-h-10 w-full truncate rounded px-2.5 py-2 text-left font-mono text-xs hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0">{recent}</button>
                    ))}
                  </>
                )}
                {bookmarkedPaths.length === 0 && recentPaths.length === 0 ? (
                  <p className="px-2.5 py-2 text-xs text-ink-muted">{t("sftp.noPlaces")}</p>
                ) : null}
              </div>
            ) : null}
            {menu?.kind === "selected" && selectedEntries.length > 0 ? (
              <div ref={menuPanel} role="menu" aria-label={selectionMenuLabel()} className="absolute right-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                <MenuActionList actions={selectedMenuActions()} />
              </div>
            ) : null}
            {menu?.kind === "context" && selectedEntries.length > 0 ? (
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
          <div className="min-h-0 min-w-0 flex-1 overflow-auto" onKeyDown={handleListKeys}>
            {alias === "" ? (
              <PanelState tone="empty" title={t("sftp.chooseHost")} detail={t("sftp.chooseHostHint")} />
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
                  <Button disabled={busy || dirty} onClick={() => { pendingFocus.current = parentRowKey; void load(parentOf(path)); }}>
                    {t("sftp.parentDirectory")}
                  </Button>
                ) : undefined}
              />
            ) : compactViewport ? (
              <ul aria-label={t("sftp.entries")} className="divide-y divide-line/40">
                {parentRowVisible ? (
                  <li data-row-key={parentRowKey}>
                    <button
                      type="button"
                      ref={(node) => { registerRow(parentRowKey, node); }}
                      tabIndex={activeRowKey === parentRowKey ? 0 : -1}
                      disabled={busy || dirty}
                      onFocus={() => setFocusedKey(parentRowKey)}
                      onClick={() => { pendingFocus.current = parentRowKey; void load(parentOf(path)); }}
                      className="flex min-h-11 w-full items-center gap-2 px-2 py-1.5 text-left text-sm hover:bg-select-fill disabled:text-ink-faint md:min-h-8 md:py-0.5"
                    >
                      <Icon name="groups" className="size-4 text-ink-muted" />
                      <span aria-hidden="true" className="font-mono">..</span>
                      <span className="sr-only">{t("sftp.parentDirectory")}</span>
                    </button>
                  </li>
                ) : null}
                {displayedEntries.map((entry) => (
                  <li
                    key={entry.path}
                    data-row-key={entry.path}
                    className={`flex items-center transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill/75" : ""}`}
                    onContextMenu={(event) => rowContextMenu(event, entry)}
                    draggable={entry.type === "file" || entry.type === "directory"}
                    onDragStart={(event) => beginRemoteDrag(event, entry)}
                  >
                    <label className="flex size-11 shrink-0 items-center justify-center md:size-8">
                      <input
                        type="checkbox"
                        aria-label={t("sftp.selectEntry", { name: entry.name })}
                        checked={selectedPaths.has(entry.path)}
                        tabIndex={activeRowKey === entry.path ? 0 : -1}
                        onChange={() => toggleSelection(entry)}
                        className="size-4 accent-accent"
                      />
                    </label>
                    <button
                      type="button"
                      ref={(node) => { registerRow(entry.path, node); }}
                      aria-label={entry.name}
                      aria-pressed={selectedPaths.has(entry.path)}
                      tabIndex={activeRowKey === entry.path ? 0 : -1}
                      className="flex min-h-11 min-w-0 grow items-center gap-2 px-2 py-1.5 text-left hover:bg-select-fill md:min-h-8 md:py-0.5"
                      onFocus={() => setFocusedKey(entry.path)}
                      onClick={(event) => clickEntry(entry, event)}
                      onDoubleClick={() => activate(entry)}
                      onPointerDown={(event) => beginLongPress(event, entry)}
                      onPointerMove={trackLongPress}
                      onPointerUp={cancelLongPress}
                      onPointerCancel={cancelLongPress}
                    >
                      <Icon name={entry.type === "directory" ? "groups" : entry.type === "symlink" ? "chevronRight" : "config"} className="size-4 text-ink-muted" />
                      <span className="min-w-0 grow">
                        <span className="block truncate font-mono text-sm font-medium leading-4 text-ink">{entry.name}</span>
                        <span className="mt-0.5 flex min-w-0 gap-2 text-[11px] leading-3 text-ink-muted">
                          <span className="truncate font-mono">{search === null ? entry.mode : parentOf(entry.path)}</span>
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
              <thead className="sticky top-0 bg-toolbar/75 text-xs text-ink-muted"><tr>
                <th scope="col" className="w-9 px-2 py-1.5 md:py-1">
                  <input
                    ref={selectAll}
                    type="checkbox"
                    aria-label={t("sftp.selectAll")}
                    checked={allDisplayedSelected}
                    onChange={toggleAllDisplayed}
                    className="size-4 accent-accent"
                  />
                </th>
                <SortableTableHeader column="name" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5 md:py-1">{t("sftp.name")}</SortableTableHeader>
                <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5 md:py-1">{t("sftp.modified")}</SortableTableHeader>
                <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="px-2 py-1.5 text-right md:py-1" buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
                <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={changeSort} className="w-24 whitespace-nowrap px-2 py-1.5 md:py-1">{t("sftp.type")}</SortableTableHeader>
                <th scope="col" className="w-28 whitespace-nowrap px-2 py-1.5 md:py-1">{t("sftp.permissions")}</th>
              </tr></thead>
              <tbody>
                {parentRowVisible ? (
                  <tr data-row-key={parentRowKey} className="border-t border-line/40 hover:bg-select-fill/60">
                    <td className="px-2 py-1 md:py-0.5" colSpan={6}>
                      <button
                        type="button"
                        ref={(node) => { registerRow(parentRowKey, node); }}
                        tabIndex={activeRowKey === parentRowKey ? 0 : -1}
                        disabled={busy || dirty}
                        onFocus={() => setFocusedKey(parentRowKey)}
                        onClick={() => { pendingFocus.current = parentRowKey; void load(parentOf(path)); }}
                        className="flex w-full items-center gap-2 rounded py-0.5 text-left text-sm focus:outline-none focus-visible:ring-1 focus-visible:ring-accent disabled:text-ink-faint"
                      >
                        <Icon name="groups" className="size-4 text-ink-muted" />
                        <span aria-hidden="true" className="font-mono">..</span>
                        <span className="sr-only">{t("sftp.parentDirectory")}</span>
                      </button>
                    </td>
                  </tr>
                ) : null}
                {displayedEntries.map((entry) => (
                  <tr
                    key={entry.path}
                    data-row-key={entry.path}
                    aria-selected={selectedPaths.has(entry.path)}
                    onDoubleClick={() => activate(entry)}
                    onContextMenu={(event) => rowContextMenu(event, entry)}
                    draggable={entry.type === "file" || entry.type === "directory"}
                    onDragStart={(event) => beginRemoteDrag(event, entry)}
                    className={`cursor-default border-t border-line/40 transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill/75" : "hover:bg-select-fill/55"}`}
                  >
                    <td className="w-9 px-2 py-1 md:py-0.5">
                      <input
                        type="checkbox"
                        aria-label={t("sftp.selectEntry", { name: entry.name })}
                        checked={selectedPaths.has(entry.path)}
                        tabIndex={activeRowKey === entry.path ? 0 : -1}
                        onChange={() => toggleSelection(entry)}
                        onDoubleClick={(event) => event.stopPropagation()}
                        className="size-4 accent-accent"
                      />
                    </td>
                    <td className="max-w-64 px-2 py-1 md:py-0.5">
                      <button
                        type="button"
                        ref={(node) => { registerRow(entry.path, node); }}
                        aria-label={entry.name}
                        aria-pressed={selectedPaths.has(entry.path)}
                        tabIndex={activeRowKey === entry.path ? 0 : -1}
                        onFocus={() => setFocusedKey(entry.path)}
                        onClick={(event) => clickEntry(entry, event)}
                        onPointerDown={(event) => beginLongPress(event, entry)}
                        onPointerMove={trackLongPress}
                        onPointerUp={cancelLongPress}
                        onPointerCancel={cancelLongPress}
                        className="flex w-full min-w-0 items-center gap-2 rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                      >
                        <Icon name={entry.type === "directory" ? "groups" : entry.type === "symlink" ? "chevronRight" : "config"} className="size-4 text-ink-muted" />
                        <span className="min-w-0 grow">
                          <span className="block truncate font-mono text-sm font-medium leading-4 text-ink">{entry.name}</span>
                          {search === null ? null : <span className="block truncate font-mono text-[10px] leading-3 text-ink-muted">{parentOf(entry.path)}</span>}
                        </span>
                      </button>
                    </td>
                    <td className="whitespace-nowrap px-2 py-1 text-xs text-ink-muted md:py-0.5">{new Date(entry.modifiedAt).toLocaleString()}</td>
                    <td className="px-2 py-1 text-right text-xs text-ink-muted md:py-0.5">{entry.type === "file" ? entry.size.toLocaleString() : "—"}</td>
                    <td className="w-24 whitespace-nowrap px-2 py-1 text-xs text-ink-muted md:py-0.5">{t(`sftp.type.${entry.type}`)}</td>
                    <td className="w-28 whitespace-nowrap px-2 py-1 font-mono text-xs text-ink-muted md:py-0.5">{entry.mode}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            )}
          </div>
          {showTransfers ? <TransferManagerList /> : null}
        </div>

      </div>

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

      {opened === null ? null : (
        <ModalShell
          labelledBy={`${headingId}-editor`}
          onDismiss={() => {
            if (dirty) setProblem(t("sftp.unsavedBlocked"));
            else setOpened(null);
          }}
          panelClassName="flex h-[min(52rem,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-lg"
        >
          <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
            <h2 id={`${headingId}-editor`} className="min-w-0 grow truncate font-mono text-xs">{opened.entry.path}</h2>
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

      {details === null ? null : (
        <SFTPDetailsDialog
          alias={alias}
          entries={details}
          busy={busy}
          returnFocusRef={activeRow}
          onClose={() => setDetails(null)}
          onEdit={(entry) => { setDetails(null); void openText(entry); }}
          onDownload={(targets) => { setDetails(null); void downloadEntries(targets); }}
          onRename={(entry) => { setDetails(null); setInputIntent({ kind: "rename", entry }); }}
        />
      )}

      {leaving === null ? null : (
        <ConfirmDialog
          id={`${headingId}-leave`}
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: opened?.entry.path ?? "" })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={() => {
            const destination = leaving;
            setLeaving(null);
            setOpened(null);
            setContents("");
            onNavigationBlockerChange?.(null);
            onNavigateLocation?.(`${destination.pathname}${destination.search}`);
          }}
          onCancel={() => setLeaving(null)}
        />
      )}

      {deleting === null ? null : (
        <ConfirmDialog
          id={`${headingId}-delete`}
          heading={deleting.length === 1 ? t("sftp.deleteHeading") : t("sftp.deleteHeadingCount", { count: deleting.length })}
          body={<ul className="max-h-48 space-y-1 overflow-auto text-sm text-ink-muted">
            {deleting.map((entry) => <li key={entry.path} className="break-all font-mono">{entry.path}</li>)}
          </ul>}
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
          heading={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "rename" ? "sftp.rename" : "sftp.chmod")}
          label={t(inputIntent.kind === "chmod" ? "sftp.chmodPrompt" : inputIntent.kind === "rename" ? "sftp.renamePrompt" : "sftp.mkdirPrompt")}
          initialValue={inputIntent.kind === "mkdir" ? "" : inputIntent.kind === "rename" ? inputIntent.entry.name : symbolicModeToOctal(inputIntent.entry.mode)}
          inputMode={inputIntent.kind === "chmod" ? "numeric" : "text"}
          submitLabel={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "rename" ? "sftp.rename" : "sftp.chmod")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={activeRow}
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

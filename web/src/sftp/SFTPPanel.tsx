import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import type { HostEntry } from "../api/config";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import { nextSort } from "../ui/tableSort";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { mobileViewportQuery, useCompactViewport, useMediaQuery } from "../ui/useMediaQuery";
import type { RemoteEntry } from "./api";
import { formatBytes } from "./format";
import { SFTPDetailsDialog } from "./SFTPDetailsDialog";
import { SFTPEntryList, sortEntries, useSFTPEntryList, type SFTPSort, type SFTPSortState } from "./SFTPEntryList";
import { SFTPTextEditor, useSFTPTextEditor } from "./SFTPTextEditor";
import { SFTPToolbar } from "./SFTPToolbar";
import { TransferManagerList } from "./TransferManagerList";
import { sftpTransferManager } from "./transferManager";
import { remoteParentOf } from "./sftpSource";
import { useSFTPBrowser, type RestoredSFTPLocation } from "./useSFTPBrowser";
import { SFTPEntryActionDialogs, useSFTPEntryActions } from "./useSFTPEntryActions";
import { useSFTPSearch } from "./useSFTPSearch";
import { useSFTPTransfers, type SFTPCounterpart } from "./useSFTPTransfers";

const noHosts: HostEntry[] = [];

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

// One pane for any source. What it offers follows the source's capabilities,
// never a check on which kind of source it is: the engine's own disk lists
// and transfers, an SSH host does everything.
export function SFTPPanel({
  aliases,
  hosts = noHosts,
  target = null,
  initialLocation = null,
  initialSort = { key: "name", direction: "ascending" },
  showTransfers = true,
  counterpart = null,
  onTargetHandled = () => undefined,
  onLocationChange = () => undefined,
  onSortChange = () => undefined,
  onNavigationBlockerChange,
  onDirtyChange,
  onNavigateLocation,
  onOpenTerminal,
  onQueueOpen,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  // Where a restored tab should reopen. Applied once, when the declared
  // aliases have arrived and can vouch for the host.
  initialLocation?: RestoredSFTPLocation | null;
  initialSort?: SFTPSortState;
  showTransfers?: boolean;
  // The other visible pane, so that files can go straight between the two.
  counterpart?: SFTPCounterpart | null;
  onTargetHandled?: (request: number) => void;
  onLocationChange?: (alias: string, path: string) => void;
  onSortChange?: (sort: SFTPSortState) => void;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onDirtyChange?: ((path: string | null) => void) | undefined;
  onNavigateLocation?: ((url: string) => void) | undefined;
  onOpenTerminal?: ((alias: string, path: string) => void | Promise<void>) | undefined;
  onQueueOpen?: () => void;
}) {
  const t = useTranslate();
  const [details, setDetails] = useState<RemoteEntry[] | null>(null);
  const [menu, setMenu] = useState<SFTPMenu | null>(null);
  const [sort, setSort] = useState<SFTPSortState>(initialSort);
  const panelRoot = useRef<HTMLElement>(null);
  const compactViewport = useCompactViewport(panelRoot);
  const mobileInteraction = useMediaQuery(mobileViewportQuery);
  const headingId = useId();
  const handledTarget = useRef(0);
  const menuRoot = useRef<HTMLDivElement>(null);
  const menuPanel = useRef<HTMLDivElement>(null);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const refreshedDeletes = useRef(new Set<string>());
  const transferJobs = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);

  useDismissibleLayer({
    open: menu !== null && !mobileInteraction,
    containerRefs: [menuRoot],
    onDismiss: () => setMenu(null),
    returnFocusRef: menuTrigger,
  });
  useMenuKeyboard({ open: menu !== null && !mobileInteraction, menuRef: menuPanel, onClose: () => setMenu(null) });

  const browser = useSFTPBrowser({
    aliases,
    initialLocation,
    onLocationChange,
    onLoaded: (listing, { refresh, changed }) => {
      list.setSelectedPaths((current) => new Set(listing.entries.filter((entry) => current.has(entry.path)).map((entry) => entry.path)));
      setMenu(null);
      search.setSearch(null);
      if (changed) actions.dismissUndo();
      if (!refresh) editor.close();
    },
    onReset: () => {
      editor.close();
      setDetails(null);
      list.clearSelection();
      list.setFocusedKey(null);
      search.clear();
      setMenu(null);
      actions.dismissUndo();
    },
  });
  const { alias, path, connected, entries, problem, setProblem, pendingPath, listingFailed, source } = browser;
  const can = source?.can;
  const local = source?.local === true;
  const editor = useSFTPTextEditor({
    onProblem: setProblem,
    // The saved revision is what the listing must show next.
    onSaved: async (targetAlias, saved) => (await browser.load(remoteParentOf(saved.entry.path), { alias: targetAlias, refresh: true })) !== null,
    onNavigationBlockerChange,
    onDirtyChange,
    onNavigateLocation,
  });
  const dirty = editor.dirty;
  const search = useSFTPSearch({
    browser,
    onResults: () => {
      list.clearSelection();
      list.setFocusedKey(null);
      setMenu(null);
      actions.dismissUndo();
    },
  });
  const displayedEntries = search.matches(sortEntries(search.listedEntries, sort));
  const parentRowVisible = search.search === null && !browser.atRoot;
  // The actions and transfers below add to busy, but they also disable the
  // rows through the list container; the selection model only needs to know
  // about reads and the editor.
  const list = useSFTPEntryList({
    entries: displayedEntries,
    loadedEntries: search.listedEntries,
    parentRowVisible,
    busy: browser.busy || editor.busy,
    locked: dirty,
    mobileInteraction,
    onActivate: (entry) => {
      if (entry.type === "directory") void load(entry.path);
      else if (can?.details) setDetails([entry]);
    },
    onOpenParent: () => { void browser.openParent(); },
    onInteract: () => setMenu(null),
    onContextMenu: (_entry, x, y) => {
      menuTrigger.current = null;
      setMenu({ kind: "context", x, y });
    },
    onRenameKey: () => { if (can?.rename) actions.renameSelection(); },
    onDeleteKey: () => { if (can?.delete) actions.deleteSelection(); },
    onEscape: search.search === null ? undefined : search.endSearch,
  });
  const { selectedPaths, selectedEntries, selectedEntry, pendingFocus, activeRow, activate, openParent } = list;
  const actions = useSFTPEntryActions({
    browser,
    list,
    refreshAfterChange: search.refreshAfterChange,
    onQueueOpen: () => transfers.openQueue(),
    onInteract: () => setMenu(null),
  });
  const transfers = useSFTPTransfers({
    browser,
    selectedEntries,
    busy: browser.busy || actions.acting || editor.busy,
    counterpart,
    onQueueOpen,
  });
  const busy = browser.busy || actions.acting || editor.busy || transfers.queuing;

  function changeSort(key: SFTPSort) {
    setSort((current) => {
      const next = nextSort(current.key, current.direction, key);
      onSortChange(next);
      return next;
    });
  }

  // Every listing from the pane closes an open menu first: the rows it acted
  // on are about to be replaced.
  function load(nextPath?: string, options?: Parameters<typeof browser.load>[1]) {
    setMenu(null);
    return browser.load(nextPath, options);
  }

  useEffect(() => {
    if (target === null || target.request === handledTarget.current) return;
    handledTarget.current = target.request;
    onTargetHandled(target.request);
    if (!aliases.includes(target.alias) || !target.path.startsWith("/") || target.path.length > 4096 || /[\x00\r\n]/u.test(target.path)) {
      setProblem(t("sftp.linkTargetInvalid"));
      return;
    }
    browser.selectHost(target.alias);
    const directory = remoteParentOf(target.path);
    void load(directory, { alias: target.alias }).then(async (loaded) => {
      if (loaded === null) return;
      const entry = loaded.find((candidate) => candidate.path === target.path);
      if (target.action === "browse") {
        if (entry?.type === "directory") await load(entry.path, { alias: target.alias });
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
      await transfers.transferOut([entry], target.alias);
    });
    // The request number makes an intentional repeat actionable while preventing route rerenders from reopening it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.request]);

  // A deletion that finished under the rows on screen (or under the search
  // root) changes them, unless an unsaved edit is holding the pane still.
  useEffect(() => {
    if (!connected || dirty || local) return;
    const completed = transferJobs.filter((job) => job.direction === "remote" && job.operation === "delete" && job.status === "completed" && job.alias === alias &&
      search.covers(job.remotePath, remoteParentOf) && !refreshedDeletes.current.has(job.id));
    if (completed.length === 0) return;
    for (const job of completed) refreshedDeletes.current.add(job.id);
    void search.refreshAfterChange(path, alias);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transferJobs, alias, path, connected, dirty, search.search, local]);

  function showDetails() {
    if (selectedEntries.length === 0 || !can?.details) return;
    setMenu(null);
    setDetails(selectedEntries);
  }

  function toggleMenu(kind: "folder" | "create" | "selected", trigger: HTMLButtonElement) {
    menuTrigger.current = trigger;
    setMenu((current) => current?.kind === kind ? null : { kind });
  }

  const transferableSelection = selectedEntries.some((entry) => entry.type === "file" || entry.type === "directory");
  // What "send it over" is called depends on where it goes: a host's files
  // download, the engine's files upload to the host in the other pane.
  const transferOutLabel = t(local ? "sftp.local.upload" : "sftp.download");

  function selectedMenuActions(): SFTPMenuAction[] {
    const items: SFTPMenuAction[] = [];
    if (selectedEntry !== null && selectedEntry.type === "directory") {
      items.push({ key: "open", label: t("sftp.openFolder"), disabled: busy || dirty, run: () => activate(selectedEntry) });
    }
    if (search.search !== null && selectedEntry !== null) {
      items.push({
        key: "reveal",
        label: t("sftp.revealInFolder"),
        disabled: busy || dirty,
        run: () => {
          setMenu(null);
          pendingFocus.current = selectedEntry.path;
          void load(remoteParentOf(selectedEntry.path));
        },
      });
    }
    if (can?.details) items.push({ key: "details", label: t("sftp.details"), disabled: busy, run: showDetails });
    if (can?.edit && selectedEntry !== null && selectedEntry.type !== "directory") {
      items.push({ key: "edit", label: t("sftp.editFile"), disabled: busy || dirty, run: () => { setMenu(null); void editor.open(alias, selectedEntry); } });
    }
    if (transferableSelection && transfers.canTransferOut) {
      items.push({ key: "download", label: transferOutLabel, disabled: busy, run: () => { setMenu(null); void transfers.transferOut(selectedEntries); } });
    }
    if (can?.chmod && selectedEntry !== null && (selectedEntry.type === "file" || selectedEntry.type === "directory")) {
      items.push({ key: "chmod", label: t("sftp.chmod"), disabled: busy, run: () => actions.ask({ kind: "chmod", entry: selectedEntry, recursive: false }) });
    }
    if (can?.chmod && selectedEntry !== null && selectedEntry.type === "directory") {
      items.push({ key: "chmodRecursive", label: t("sftp.chmodRecursive"), disabled: busy, run: () => actions.ask({ kind: "chmod", entry: selectedEntry, recursive: true }) });
    }
    if (can?.rename && selectedEntry !== null) {
      items.push({ key: "rename", label: t("sftp.rename"), disabled: busy, run: actions.renameSelection });
    }
    if (can?.createEntries && selectedEntry !== null && (selectedEntry.type === "file" || selectedEntry.type === "directory")) {
      items.push({ key: "duplicate", label: t("sftp.duplicate"), disabled: busy, run: () => actions.ask({ kind: "duplicate", entry: selectedEntry }) });
    }
    if (can?.rename && selectedEntries.length > 0 && selectedEntries.every((entry) => entry.type === "file" || entry.type === "directory")) {
      items.push({ key: "moveTo", label: t("sftp.moveTo"), disabled: busy, run: () => actions.ask({ kind: "moveTo", entries: selectedEntries }) });
    }
    items.push({ key: "copyName", label: t(selectedEntries.length === 1 ? "sftp.copyName" : "sftp.copyNames"), run: () => void actions.copySelected("name") });
    items.push({ key: "copyPath", label: t(selectedEntries.length === 1 ? "sftp.copyPath" : "sftp.copyPaths"), run: () => void actions.copySelected("path") });
    if (can?.delete) items.push({ key: "delete", label: t("sftp.delete"), danger: true, disabled: busy, run: actions.deleteSelection });
    items.push({ key: "invert", label: t("sftp.invertSelection"), run: list.invertDisplayedSelection });
    items.push({ key: "clear", label: t("sftp.clearSelection"), run: () => { setMenu(null); list.clearSelection(); } });
    return items;
  }

  function selectionMenuLabel(): string {
    return selectedEntry === null
      ? t("sftp.selectedActionsCount", { count: selectedEntries.length })
      : t("sftp.selectedActions", { name: selectedEntry.name });
  }

  function folderMenuActions(): SFTPMenuAction[] {
    return [
      { key: "copyCurrentPath", label: t("sftp.copyPath"), disabled: !connected, run: () => { setMenu(null); void actions.copyCurrentPath(); } },
      ...(can?.createEntries ? [
        { key: "newFolder", label: t("sftp.newFolder"), disabled: busy || !connected, run: () => actions.ask({ kind: "mkdir" }) },
        { key: "newFile", label: t("sftp.newFile"), disabled: busy || !connected, run: () => actions.ask({ kind: "createFile" }) },
      ] : []),
      ...(can?.browserUpload ? [
        { key: "upload", label: t("sftp.upload"), disabled: busy || !connected, run: () => { setMenu(null); transfers.chooseFiles(); } },
        { key: "uploadFolder", label: t("sftp.uploadFolder"), disabled: busy || !connected, run: () => { setMenu(null); transfers.chooseFolder(); } },
      ] : []),
      { key: "forward", label: t("sftp.forward"), disabled: busy || dirty || !browser.canForward, run: () => { setMenu(null); void browser.forward(); } },
      { key: "home", label: t("sftp.homeDirectory"), disabled: busy || dirty || !connected, run: () => { void load(""); } },
      { key: "root", label: t("sftp.rootDirectory"), disabled: busy || dirty || !connected || browser.atRoot, run: () => { setMenu(null); void browser.goRoot(); } },
      ...(can?.terminal && onOpenTerminal !== undefined ? [{ key: "terminal", label: t("sftp.openTerminalHere"), disabled: busy || dirty || !connected, run: () => { setMenu(null); void onOpenTerminal(alias, path); } }] : []),
      { key: "selectAll", label: t("sftp.selectAll"), disabled: busy || displayedEntries.length === 0, run: list.selectAllDisplayed },
      ...(["name", "type", "size", "modified"] as const).map((key) => ({
        key: `sort-${key}`,
        label: `${t(`sftp.${key}`)}${t(sort.key === key && sort.direction === "ascending" ? "table.sortDescending" : "table.sortAscending")}`,
        run: () => { changeSort(key); setMenu(null); },
      })),
    ];
  }

  const filterInput = (
    <input
      type="search"
      aria-label={t("sftp.filter")}
      value={search.filter}
      onChange={(event) => search.setFilter(event.target.value)}
      onKeyDown={(event) => {
        if (event.key !== "Enter" || !can?.search) return;
        event.preventDefault();
        void search.runSearch();
      }}
      placeholder={t("sftp.filterPlaceholder")}
      className="h-8 w-full rounded-md border border-control-line/60 bg-control/70 py-1 pl-7 pr-2 text-xs outline-none focus:border-accent md:h-7"
    />
  );
  const loadingLabel = t(local ? "sftp.local.loading" : "sftp.loading");

  return (
    <section ref={panelRoot} className="flex h-full min-h-0 min-w-0 flex-col gap-1.5 md:gap-1" aria-labelledby={headingId}>
      <h2 id={headingId} className="sr-only">{t(local ? "sftp.local.heading" : "sftp.heading")}</h2>
      <SFTPToolbar
        browser={browser}
        aliases={aliases}
        hosts={hosts}
        onHostChange={browser.selectHost}
        onRefresh={() => { if (!busy && !dirty && connected) search.refreshCurrent(); }}
        busy={busy}
        locked={dirty}
        mobile={mobileInteraction}
        labels={local
          ? { path: t("sftp.local.path"), editPath: t("sftp.local.editPath"), input: t("sftp.local.pathInput") }
          : { path: t("sftp.path"), editPath: t("sftp.editPath"), input: t("sftp.path") }}
        leading={can?.terminal && onOpenTerminal !== undefined ? (
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
        ) : null}
        mobileActions={<>
          <button type="button" aria-label={t("sftp.mobile.search")} aria-expanded={search.mobileSearchOpen} disabled={busy || !connected} onClick={() => search.setMobileSearchOpen((value) => !value)} className={`flex size-11 shrink-0 items-center justify-center rounded active:bg-select-fill ${search.mobileSearchOpen || search.filter !== "" ? "text-accent" : "text-ink-muted"}`}><Icon name="search" className="size-4" /></button>
          <button type="button" aria-label={t("sftp.mobile.actions")} aria-haspopup="dialog" aria-expanded={menu?.kind === "folder"} onClick={(event) => toggleMenu("folder", event.currentTarget)} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="moreHorizontal" className="size-4" /></button>
        </>}
      />

      {problem === "" || listingFailed ? null : <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}
      {search.search === null ? null : (
        <p role="status" className="flex items-center gap-3 rounded-md border border-line bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
          <span className="min-w-0 grow truncate">
            {t(search.search.truncated ? "sftp.searchResultsTruncated" : "sftp.searchResults", {
              count: search.search.entries.length,
              query: search.search.query,
              path: search.search.root,
            })}
          </span>
          <button type="button" disabled={busy} onClick={search.endSearch} className="shrink-0 text-accent disabled:text-ink-faint">{t("sftp.searchEnd")}</button>
        </p>
      )}
      {actions.undo === null ? null : (
        <p role="status" className="flex items-center gap-3 rounded-md border border-line bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
          <span className="min-w-0 grow truncate">{actions.undo.label}</span>
          <button type="button" disabled={busy} onClick={() => void actions.undo?.run()} className="shrink-0 text-accent disabled:text-ink-faint">{t("sftp.undo")}</button>
          <button type="button" aria-label={t("sftp.dismissUndo")} onClick={actions.dismissUndo} className="flex size-6 shrink-0 items-center justify-center rounded text-ink-muted hover:text-ink">
            <Icon name="close" className="size-3" />
          </button>
        </p>
      )}

      <div className="grid min-h-0 min-w-0 flex-1 grid-cols-1 gap-2">
        <div
          aria-label={t(local ? "sftp.local.dropZone" : "sftp.dropZone")}
          aria-busy={busy}
          className={`relative flex min-h-0 min-w-0 flex-col rounded-md border bg-card transition-shadow ${transfers.dragging ? "border-accent ring-1 ring-accent" : "border-line/60"}`}
          onDragEnter={transfers.dragEnter}
          onDragOver={transfers.dragOver}
          onDragLeave={transfers.dragLeave}
          onDrop={(event) => { void transfers.acceptDrop(event); }}
        >
          <div ref={menuRoot} hidden={mobileInteraction && selectedEntries.length === 0 && !search.mobileSearchOpen} className={mobileInteraction && selectedEntries.length === 0 && !search.mobileSearchOpen ? "hidden" : "relative flex min-h-10 shrink-0 items-center gap-1 border-b border-line/50 bg-toolbar/45 px-2 py-1 md:min-h-8 md:py-0.5"}>
            {selectedEntries.length > 0 && !(mobileInteraction && search.mobileSearchOpen) ? (
              <>
                <button type="button" aria-label={t("sftp.clearSelection")} onClick={list.clearSelection} className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink md:size-7">
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
                  {filterInput}
                </label>
                {compactViewport || !can?.details ? null : <button type="button" disabled={busy} onClick={showDetails} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.details")}</button>}
                {compactViewport && !local ? null : <button type="button" disabled={busy || !transferableSelection || !transfers.canTransferOut} title={transfers.canTransferOut ? undefined : t("sftp.local.connectRemote")} onClick={() => void transfers.transferOut(selectedEntries)} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{transferOutLabel}</button>}
                {compactViewport || selectedEntry === null || !can?.rename ? null : <button type="button" disabled={busy} onClick={actions.renameSelection} className="rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">{t("sftp.rename")}</button>}
                {compactViewport || !can?.delete ? null : <button type="button" disabled={busy} onClick={actions.deleteSelection} className="rounded px-2 py-1 text-xs text-danger hover:bg-hover disabled:text-ink-faint">{t("sftp.delete")}</button>}
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
                <input ref={search.searchInput} type="search" aria-label={t("sftp.filter")} value={search.filter} onChange={(event) => search.setFilter(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); if (can?.search) void search.runSearch(); event.currentTarget.blur(); } }} placeholder={t("sftp.filterPlaceholder")} className="h-11 min-w-0 flex-1 rounded-md border border-control-line bg-control px-3 text-base" />
                {can?.search ? <button type="button" aria-label={t("sftp.searchBelow")} disabled={busy || !connected || search.filter.trim() === ""} onClick={() => { search.searchInput.current?.blur(); void search.runSearch(); }} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted disabled:text-ink-faint"><Icon name="search" className="size-4" /></button> : null}
                <button type="button" aria-label={t("sftp.close")} onClick={() => { search.setMobileSearchOpen(false); search.setFilter(""); }} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted"><Icon name="close" className="size-4" /></button>
              </>
            ) : (
            <>
            {can?.createEntries || can?.browserUpload ? (
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
            ) : null}
            <label className="relative min-w-24 max-w-52 grow">
              <span className="sr-only">{t("sftp.filter")}</span>
              <Icon name="search" className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-ink-muted" />
              {filterInput}
            </label>
            {can?.search ? (
              <button
                type="button"
                aria-label={t("sftp.searchBelow")}
                title={t("sftp.searchBelow")}
                disabled={busy || !connected || search.filter.trim() === ""}
                onClick={() => void search.runSearch()}
                className="flex size-10 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:size-7"
              >
                <Icon name="search" className="size-4" />
              </button>
            ) : null}
            </>
            )}
            {can?.browserUpload ? (
              <>
                <input
                  ref={transfers.uploadInput}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(event) => transfers.uploadFromInput(event, (file) => file.name)}
                />
                <input
                  ref={(element) => { transfers.folderUploadInput.current = element; element?.setAttribute("webkitdirectory", ""); }}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(event) => transfers.uploadFromInput(event, (file) => (file as File & { webkitRelativePath?: string }).webkitRelativePath ?? file.name)}
                />
              </>
            ) : null}
            {!mobileInteraction && menu?.kind === "create" ? (
              <div ref={menuPanel} role="menu" aria-label={t("sftp.createActions")} className="absolute left-2 top-full z-20 mt-1 w-52 rounded-lg border border-control-line bg-card p-1 shadow-lg">
                {can?.createEntries ? <>
                  <button type="button" role="menuitem" disabled={busy} onClick={() => actions.ask({ kind: "mkdir" })} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFolder")}</button>
                  <button type="button" role="menuitem" disabled={busy} onClick={() => actions.ask({ kind: "createFile" })} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.newFile")}</button>
                </> : null}
                {can?.browserUpload ? <>
                  <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); transfers.chooseFiles(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.upload")}</button>
                  <button type="button" role="menuitem" disabled={busy} onClick={() => { setMenu(null); transfers.chooseFolder(); }} className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0">{t("sftp.uploadFolder")}</button>
                </> : null}
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
          {connected && pendingPath !== null ? <div role="status" className="absolute inset-0 z-10 flex items-center justify-center rounded-md bg-card/80 p-4 backdrop-blur-[1px]"><span className="flex min-w-0 items-center gap-3 rounded-lg bg-toolbar px-4 py-3 text-sm"><span aria-hidden="true" className="size-4 shrink-0 animate-spin rounded-full border-2 border-accent border-t-transparent motion-reduce:animate-none" /><span className="min-w-0"><span className="block">{loadingLabel}</span><span className="block truncate font-mono text-xs text-ink-muted">{pendingPath || t("sftp.homeDirectory")}</span></span></span></div> : null}
          <div data-testid="sftp-file-list" className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain" inert={connected && busy} onKeyDown={list.handleListKeys}>
            {alias === "" || can === undefined ? (
              <PanelState tone="empty" title={t("sftp.chooseHost")} detail={t("sftp.chooseHostHint")} />
            ) : !connected && busy ? (
              <PanelState tone="loading" title={can.connect ? t("sftp.connecting", { alias }) : loadingLabel} />
            ) : !connected && problem !== "" ? (
              <PanelState
                tone="failed"
                title={problem}
                action={<Button onClick={() => void browser.retry()}>{t("sftp.retry")}</Button>}
              />
            ) : !connected && can.connect ? (
              <PanelState
                tone="empty"
                title={t("sftp.readyToConnect", { alias })}
                detail={t("sftp.connectHint")}
                action={<Button kind="primary" onClick={() => void browser.connect()}>{t("sftp.connect")}</Button>}
              />
            ) : !connected || (busy && entries.length === 0 && problem === "") ? (
              <PanelState tone="loading" title={loadingLabel} />
            ) : listingFailed ? (
              <PanelState
                tone="failed"
                title={problem}
                action={<Button onClick={() => void browser.retry()}>{t("sftp.retry")}</Button>}
              />
            ) : search.search !== null && displayedEntries.length === 0 ? (
              <PanelState
                tone="empty"
                title={t("sftp.searchNoMatches", { query: search.search.query })}
                action={<Button onClick={search.endSearch}>{t("sftp.searchEnd")}</Button>}
              />
            ) : displayedEntries.length === 0 ? (
              <PanelState
                tone="empty"
                title={t(search.normalizedFilter === "" ? "sftp.emptyDirectory" : "sftp.noFilterMatches")}
                {...(search.normalizedFilter === "" ? {} : { detail: t("sftp.clearFilterHint") })}
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
                draggable={(entry) => can.dragOut && (entry.type === "file" || entry.type === "directory")}
                onDragStart={(event, entry) => transfers.beginDrag(event, entry, selectedPaths)}
                entryContext={search.search === null ? undefined : (entry) => remoteParentOf(entry.path)}
              />
            )}
          </div>
          {showTransfers ? <TransferManagerList openRequest={transfers.openQueueRequest} /> : null}
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

      {transfers.remoteDrop === null ? null : (
        <ModalShell labelledBy={`${headingId}-remote-drop`} onDismiss={transfers.dismissRemoteDrop} panelClassName="w-full max-w-md rounded-lg p-5">
          <h3 id={`${headingId}-remote-drop`} className="text-base font-semibold text-ink">{t("sftp.remoteDropTitle")}</h3>
          <p className="mt-2 text-sm leading-6 text-ink-muted">
            {t("sftp.remoteDropDescription", { count: transfers.remoteDrop.entries.length, alias })}
          </p>
          <div className="mt-5 flex flex-wrap justify-end gap-2">
            <Button onClick={transfers.dismissRemoteDrop}>{t("sftp.cancel")}</Button>
            <Button onClick={() => void transfers.acceptRemoteDrop("move")}>{t("sftp.moveHere")}</Button>
            <Button kind="primary" onClick={() => void transfers.acceptRemoteDrop("copy")}>{t("sftp.copyHere")}</Button>
          </div>
        </ModalShell>
      )}

      <SFTPTextEditor editor={editor} busy={browser.busy || actions.acting || transfers.queuing} />

      {details === null ? null : (
        <SFTPDetailsDialog
          alias={alias}
          entries={details}
          busy={busy}
          returnFocusRef={activeRow}
          onClose={() => setDetails(null)}
          onEdit={(entry) => { setDetails(null); void editor.open(alias, entry); }}
          onDownload={(targets) => { setDetails(null); void transfers.transferOut(targets); }}
          onRename={(entry) => { setDetails(null); actions.ask({ kind: "rename", entry }); }}
        />
      )}

      <SFTPEntryActionDialogs actions={actions} currentPath={path} returnFocusRef={activeRow} />
    </section>
  );
}

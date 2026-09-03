import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { HostEntry } from "../api/config";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { useCompactViewport } from "../ui/useMediaQuery";
import { SFTPPanel, type SFTPTarget } from "./SFTPPanel";
import { SFTPCompareDialog } from "./SFTPCompareDialog";
import { TransferManagerList } from "./TransferManagerList";

const storageKey = "sshc.sftp.tabs";
const activeStorageKey = "sshc.sftp.activeTab";
const splitStorageKey = "sshc.sftp.split";
const secondaryStorageKey = "sshc.sftp.secondary";
const secondaryTabsStorageKey = "sshc.sftp.secondaryTabs";
const secondaryActiveStorageKey = "sshc.sftp.secondaryActiveTab";
const maxTabs = 8;

type SFTPTab = { id: string; alias: string; path: string };
type SFTPLocation = { alias: string; path: string };
type SFTPPane = "primary" | "secondary";
type CloseTabIntent = { pane: SFTPPane; id: string; path: string };

function tabStateKey(pane: SFTPPane, id: string): string {
  return `${pane}:${id}`;
}

function identifier(): string {
  return globalThis.crypto?.randomUUID?.() ?? `tab_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function blankTab(): SFTPTab {
  return { id: identifier(), alias: "", path: "" };
}

function restoreTabs(key: string, blankWhenEmpty: boolean): SFTPTab[] {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(key) ?? "[]");
    if (!Array.isArray(raw)) return blankWhenEmpty ? [blankTab()] : [];
    const tabs = raw.flatMap((value): SFTPTab[] => {
      if (typeof value !== "object" || value === null) return [];
      const tab = value as Record<string, unknown>;
      const alias = typeof tab.alias === "string" ? tab.alias : "";
      const path = typeof tab.path === "string" && tab.path.startsWith("/") ? tab.path : "";
      return [{ id: identifier(), alias, path }];
    }).slice(0, maxTabs);
    return tabs.length === 0 && blankWhenEmpty ? [blankTab()] : tabs;
  } catch {
    return blankWhenEmpty ? [blankTab()] : [];
  }
}

function restorePrimaryTabs(): SFTPTab[] {
  return restoreTabs(storageKey, true);
}

function restoreSecondaryTabs(): SFTPTab[] {
  const tabs = restoreTabs(secondaryTabsStorageKey, false);
  if (tabs.length > 0) return tabs;
  const legacy = restoreSecondary();
  return legacy.alias === "" && legacy.path === "" ? [] : [{ id: identifier(), ...legacy }];
}

function rememberTabs(key: string, tabs: SFTPTab[]): void {
  try {
    window.localStorage.setItem(key, JSON.stringify(tabs.map(({ alias, path }) => ({ alias, path }))));
  } catch {
    // A browser that refuses storage still keeps the tabs for this session.
  }
}

function restoreActive(key: string, tabs: SFTPTab[]): string {
  try {
    const index = Number.parseInt(window.localStorage.getItem(key) ?? "0", 10);
    return tabs[Number.isInteger(index) && index >= 0 && index < tabs.length ? index : 0]?.id ?? "";
  } catch {
    return tabs[0]?.id ?? "";
  }
}

function rememberActive(key: string, id: string, tabs: SFTPTab[]): void {
  try {
    const index = tabs.findIndex((tab) => tab.id === id);
    window.localStorage.setItem(key, String(index < 0 ? 0 : index));
  } catch {
    // Storage is a convenience. The selected tab still works for this session.
  }
}

function restoreSplit(): boolean {
  try {
    return window.localStorage.getItem(splitStorageKey) === "true";
  } catch {
    return false;
  }
}

function restoreSecondary(): SFTPLocation {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(secondaryStorageKey) ?? "{}");
    if (typeof raw !== "object" || raw === null) return { alias: "", path: "" };
    const value = raw as Record<string, unknown>;
    return {
      alias: typeof value.alias === "string" ? value.alias : "",
      path: typeof value.path === "string" && value.path.startsWith("/") ? value.path : "",
    };
  } catch {
    return { alias: "", path: "" };
  }
}

function rememberSplit(split: boolean): void {
  try {
    window.localStorage.setItem(splitStorageKey, String(split));
  } catch {
    // Storage is a convenience. The open panes still work for this session.
  }
}

function tabLabel(tab: SFTPTab, unnamed: string): string {
  if (tab.alias === "") return unnamed;
  if (tab.path === "" || tab.path === "/") return tab.alias;
  const name = tab.path.split("/").filter(Boolean).pop() ?? tab.path;
  return `${tab.alias}:${name}`;
}

function activeTab(tabs: SFTPTab[], activeId: string): SFTPTab | undefined {
  return tabs.find((tab) => tab.id === activeId) ?? tabs[0];
}

// Both panes own the same tab model. Each visible tab keeps its own host,
// history and selection, while the transfer queue remains global to the
// engine and is drawn only once below the panes.
export function SFTPWorkspace({
  aliases,
  hosts,
  target = null,
  onTargetHandled = () => undefined,
  onNavigationBlockerChange,
  onNavigateLocation,
  onOpenTerminal,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  onTargetHandled?: (request: number) => void;
  onNavigationBlockerChange?: (blocker: NavigationBlocker | null) => void;
  onNavigateLocation?: (url: string) => void;
  onOpenTerminal?: (alias: string, path: string) => void | Promise<void>;
}) {
  const t = useTranslate();
  const [tabs, setTabs] = useState<SFTPTab[]>(restorePrimaryTabs);
  const [activeId, setActiveId] = useState(() => restoreActive(activeStorageKey, tabs));
  const [split, setSplit] = useState(restoreSplit);
  const [secondaryTabs, setSecondaryTabs] = useState<SFTPTab[]>(() => {
    const restored = restoreSecondaryTabs();
    if (restored.length > 0 || !restoreSplit()) return restored;
    const source = activeTab(tabs, activeId) ?? blankTab();
    return [{ ...source, id: identifier() }];
  });
  const [secondaryActiveId, setSecondaryActiveId] = useState(() => restoreActive(secondaryActiveStorageKey, secondaryTabs));
  const [focusedPane, setFocusedPane] = useState<SFTPPane>("primary");
  const [compareOpen, setCompareOpen] = useState(false);
  const [dirtyTabs, setDirtyTabs] = useState<Map<string, string>>(() => new Map());
  const [closeTabIntent, setCloseTabIntent] = useState<CloseTabIntent | null>(null);
  const workspaceRoot = useRef<HTMLElement | null>(null);
  const compactViewport = useCompactViewport(workspaceRoot);
  // Restoring is a one-shot per tab: once a panel has opened its remembered
  // directory, later navigation inside it must not be pulled back.
  const restoring = useRef<Record<SFTPPane, Map<string, SFTPLocation>>>({
    primary: new Map(tabs.map((tab) => [tab.id, { alias: tab.alias, path: tab.path }])),
    secondary: new Map(secondaryTabs.map((tab) => [tab.id, { alias: tab.alias, path: tab.path }])),
  });
  const tabScrollers = useRef<Record<SFTPPane, HTMLDivElement | null>>({ primary: null, secondary: null });
  const blockers = useRef<{ primary: NavigationBlocker | null; secondary: NavigationBlocker | null }>({ primary: null, secondary: null });
  const dirtyReporters = useRef(new Map<string, (path: string | null) => void>());
  const active = tabs.some((tab) => tab.id === activeId) ? activeId : tabs[0]?.id ?? "";
  const secondaryActive = secondaryTabs.some((tab) => tab.id === secondaryActiveId)
    ? secondaryActiveId
    : secondaryTabs[0]?.id ?? "";
  const visibleSplit = split && !compactViewport;

  useEffect(() => {
    for (const pane of (["primary", "secondary"] as const)) {
      if (pane === "secondary" && !visibleSplit) continue;
      const selected = tabScrollers.current[pane]?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
      selected?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    }
  }, [active, secondaryActive, visibleSplit]);

  useEffect(() => {
    if (!compactViewport) return;
    setFocusedPane("primary");
    setCompareOpen(false);
  }, [compactViewport]);

  function commit(pane: SFTPPane, next: SFTPTab[]) {
    if (pane === "primary") {
      setTabs(next);
      rememberTabs(storageKey, next);
      return;
    }
    setSecondaryTabs(next);
    rememberTabs(secondaryTabsStorageKey, next);
  }

  function relocate(pane: SFTPPane, id: string, alias: string, path: string) {
    restoring.current[pane].delete(id);
    const update = (current: SFTPTab[]) => {
      const next = current.map((tab) => tab.id === id ? { ...tab, alias, path } : tab);
      if (next.every((tab, index) => tab.alias === current[index]?.alias && tab.path === current[index]?.path)) {
        return current;
      }
      rememberTabs(pane === "primary" ? storageKey : secondaryTabsStorageKey, next);
      return next;
    };
    if (pane === "primary") setTabs(update);
    else setSecondaryTabs(update);
  }

  function addTab(pane: SFTPPane) {
    const current = pane === "primary" ? tabs : secondaryTabs;
    if (current.length >= maxTabs) return;
    const opened = blankTab();
    const next = [...current, opened];
    commit(pane, next);
    if (pane === "primary") setActiveId(opened.id);
    else setSecondaryActiveId(opened.id);
    rememberActive(pane === "primary" ? activeStorageKey : secondaryActiveStorageKey, opened.id, next);
    setFocusedPane(pane);
  }

  function closeTab(pane: SFTPPane, id: string) {
    const current = pane === "primary" ? tabs : secondaryTabs;
    const currentActive = pane === "primary" ? active : secondaryActive;
    const index = current.findIndex((tab) => tab.id === id);
    if (index < 0) return;
    const stateKey = tabStateKey(pane, id);
    setDirtyTabs((current) => {
      if (!current.has(stateKey)) return current;
      const next = new Map(current);
      next.delete(stateKey);
      return next;
    });
    dirtyReporters.current.delete(stateKey);
    restoring.current[pane].delete(id);
    const remaining = current.filter((tab) => tab.id !== id);
    const next = remaining.length === 0 ? [blankTab()] : remaining;
    commit(pane, next);
    const activeKey = pane === "primary" ? activeStorageKey : secondaryActiveStorageKey;
    if (id !== currentActive) {
      rememberActive(activeKey, currentActive, next);
      return;
    }
    const nextActive = (next[Math.min(index, next.length - 1)] ?? next[0])?.id ?? "";
    if (pane === "primary") setActiveId(nextActive);
    else setSecondaryActiveId(nextActive);
    rememberActive(activeKey, nextActive, next);
  }

  const updateDirtyTab = useCallback((key: string, path: string | null) => {
    setDirtyTabs((current) => {
      if (path === null) {
        if (!current.has(key)) return current;
        const next = new Map(current);
        next.delete(key);
        return next;
      }
      if (current.get(key) === path) return current;
      return new Map(current).set(key, path);
    });
  }, []);

  const dirtyReporter = useCallback((pane: SFTPPane, id: string) => {
    const key = tabStateKey(pane, id);
    const existing = dirtyReporters.current.get(key);
    if (existing !== undefined) return existing;
    const created = (path: string | null) => updateDirtyTab(key, path);
    dirtyReporters.current.set(key, created);
    return created;
  }, [updateDirtyTab]);

  function requestCloseTab(pane: SFTPPane, tab: SFTPTab) {
    const dirtyPath = dirtyTabs.get(tabStateKey(pane, tab.id));
    if (dirtyPath !== undefined) {
      setCloseTabIntent({ pane, id: tab.id, path: dirtyPath });
      return;
    }
    closeTab(pane, tab.id);
  }

  function switchTab(pane: SFTPPane, id: string) {
    if (pane === "primary") setActiveId(id);
    else setSecondaryActiveId(id);
    rememberActive(pane === "primary" ? activeStorageKey : secondaryActiveStorageKey, id, pane === "primary" ? tabs : secondaryTabs);
    setFocusedPane(pane);
  }

  function moveWithKeyboard(pane: SFTPPane, event: ReactKeyboardEvent<HTMLButtonElement>, index: number) {
    const current = pane === "primary" ? tabs : secondaryTabs;
    activateTabFromKeyboard(event, index, current, (tab) => switchTab(pane, tab.id));
  }

  function toggleSplit() {
    const next = !split;
    if (next && secondaryTabs.length === 0) {
      const source = activeTab(tabs, active) ?? blankTab();
      const copy = { ...source, id: identifier() };
      restoring.current.secondary.set(copy.id, { alias: copy.alias, path: copy.path });
      setSecondaryTabs([copy]);
      setSecondaryActiveId(copy.id);
      rememberTabs(secondaryTabsStorageKey, [copy]);
      rememberActive(secondaryActiveStorageKey, copy.id, [copy]);
    }
    if (!next) setFocusedPane("primary");
    setSplit(next);
    rememberSplit(next);
  }

  const updateBlocker = useCallback((pane: "primary" | "secondary", blocker: NavigationBlocker | null) => {
    blockers.current[pane] = blocker;
    const current = blockers.current;
    if (current.primary === null && current.secondary === null) {
      onNavigationBlockerChange?.(null);
      return;
    }
    onNavigationBlockerChange?.((next) => {
      if (current.primary !== null && !current.primary(next)) return false;
      if (current.secondary !== null && !current.secondary(next)) return false;
      return true;
    });
  }, [onNavigationBlockerChange]);

  const updatePrimaryBlocker = useCallback((blocker: NavigationBlocker | null) => {
    updateBlocker("primary", blocker);
  }, [updateBlocker]);
  const updateSecondaryBlocker = useCallback((blocker: NavigationBlocker | null) => {
    updateBlocker("secondary", blocker);
  }, [updateBlocker]);

  const primaryLocation = activeTab(tabs, active);
  const secondaryLocation = activeTab(secondaryTabs, secondaryActive);

  function renderTabs(pane: SFTPPane) {
    const current = pane === "primary" ? tabs : secondaryTabs;
    const currentActive = pane === "primary" ? active : secondaryActive;
    return (
      <div data-sftp-pane-tabs={pane} className="flex min-w-0 flex-1 items-stretch">
        <div
          ref={(node) => { tabScrollers.current[pane] = node; }}
          role="tablist"
          aria-label={t(pane === "primary" ? "sftp.primaryTabs" : "sftp.secondaryTabs")}
          className="flex min-w-0 flex-1 items-stretch overflow-x-auto overscroll-x-contain"
        >
          {current.map((tab, index) => {
            const label = tabLabel(tab, t("sftp.newTab"));
            const selected = tab.id === currentActive;
            return (
              <span
                key={tab.id}
                className={`group relative flex shrink-0 items-center border-b-2 ${selected ? "border-accent bg-select-fill/40" : "border-transparent hover:bg-toolbar/60"}`}
              >
                <button
                  type="button"
                  role="tab"
                  id={`sftp-${pane}-tab-${tab.id}`}
                  aria-selected={selected}
                  aria-controls={`sftp-${pane}-tabpanel-${tab.id}`}
                  tabIndex={selected ? 0 : -1}
                  onClick={() => switchTab(pane, tab.id)}
                  onKeyDown={(event) => moveWithKeyboard(pane, event, index)}
                  className={`min-h-12 max-w-64 truncate px-4 py-3 text-left text-sm ${selected ? "font-medium text-ink" : "text-ink-muted"}`}
                >
                  {label}
                </button>
                {current.length > 1 ? (
                  <button
                    type="button"
                    aria-label={t("sftp.closeTab", { name: label })}
                    onClick={() => requestCloseTab(pane, tab)}
                    className="flex size-12 items-center justify-center text-ink-faint hover:text-danger md:w-9"
                  >
                    <Icon name="close" className="size-3" />
                  </button>
                ) : null}
              </span>
            );
          })}
        </div>
        <button
          type="button"
          aria-label={t("sftp.newTab")}
          disabled={current.length >= maxTabs}
          onClick={() => addTab(pane)}
          className="flex size-12 shrink-0 items-center justify-center text-ink-muted hover:bg-toolbar hover:text-ink disabled:text-ink-faint"
        >
          <Icon name="plus" className="size-4" />
        </button>
      </div>
    );
  }

  function renderPane(pane: SFTPPane, concealed = false) {
    const current = pane === "primary" ? tabs : secondaryTabs;
    const currentActive = pane === "primary" ? active : secondaryActive;
    const blocker = pane === "primary" ? updatePrimaryBlocker : updateSecondaryBlocker;
    return (
      <div
        className="flex min-h-0 min-w-0 flex-col"
        hidden={concealed}
        aria-label={t(pane === "primary" ? "sftp.firstPane" : "sftp.secondPane")}
        onPointerDown={() => setFocusedPane(pane)}
        onFocusCapture={() => setFocusedPane(pane)}
      >
        {current.map((tab) => {
          const selected = tab.id === currentActive;
          const restored = restoring.current[pane].get(tab.id);
          const ownsTarget = selected && (compactViewport ? pane === "primary" : focusedPane === pane);
          return (
            <div
              key={tab.id}
              id={`sftp-${pane}-tabpanel-${tab.id}`}
              role="tabpanel"
              aria-labelledby={`sftp-${pane}-tab-${tab.id}`}
              hidden={!selected}
              className={selected ? "flex min-h-0 min-w-0 flex-1 flex-col" : ""}
            >
              <SFTPPanel
                aliases={aliases}
                {...(hosts === undefined ? {} : { hosts })}
                target={ownsTarget ? target : null}
                initialLocation={restored === undefined || restored.alias === "" || restored.path === "" ? null : restored}
                showTransfers={false}
                {...(selected ? { onNavigationBlockerChange: blocker } : {})}
                onDirtyChange={dirtyReporter(pane, tab.id)}
                {...(onNavigateLocation === undefined ? {} : { onNavigateLocation })}
                {...(onOpenTerminal === undefined ? {} : { onOpenTerminal })}
                {...(ownsTarget ? { onTargetHandled } : {})}
                onLocationChange={(alias, path) => relocate(pane, tab.id, alias, path)}
              />
            </div>
          );
        })}
      </div>
    );
  }

  function renderPaneActions() {
    return (
      <>
        <button
          type="button"
          aria-label={t("sftp.compare.heading")}
          title={t("sftp.compare.heading")}
          disabled={!visibleSplit || primaryLocation?.alias === "" || secondaryLocation?.alias === ""}
          onClick={() => setCompareOpen(true)}
          className="mr-1 hidden h-9 shrink-0 self-center items-center gap-1.5 rounded-md px-3 text-sm text-ink-muted hover:bg-toolbar hover:text-ink disabled:text-ink-faint lg:flex"
        >
          <span aria-hidden="true">⇄</span>
          {t("sftp.compare.action")}
        </button>
        <button
          type="button"
          aria-pressed={split}
          aria-label={t(split ? "sftp.singlePane" : "sftp.splitPane")}
          title={t(split ? "sftp.singlePane" : "sftp.splitPane")}
          onClick={toggleSplit}
          className={`mr-1 hidden h-9 shrink-0 self-center items-center gap-1.5 rounded-md px-3 text-sm lg:flex ${split ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-toolbar hover:text-ink"}`}
        >
          <Icon name="inspector" className="size-3.5" />
          {t(split ? "sftp.singlePane" : "sftp.splitPane")}
        </button>
      </>
    );
  }

  return (
    <section ref={workspaceRoot} className="flex h-full min-h-0 min-w-0 flex-col" aria-label={t("sftp.tabs")}>
      <div className="flex min-h-12 shrink-0 items-stretch border-b border-line/60">
        <div className={`flex min-w-0 ${visibleSplit ? "w-1/2 flex-none" : "flex-1"}`}>
          {renderTabs("primary")}
        </div>
        {secondaryTabs.length > 0 ? (
          <div hidden={!visibleSplit} className="w-1/2 min-w-0 flex-none border-l border-line/60">
            <div className="flex min-w-0">
              {renderTabs("secondary")}
              {visibleSplit ? renderPaneActions() : null}
            </div>
          </div>
        ) : null}
        {visibleSplit || compactViewport ? null : renderPaneActions()}
      </div>

      <div className={`grid min-h-0 min-w-0 flex-1 gap-2 pt-2 ${visibleSplit ? "grid-cols-2" : "grid-cols-1"}`}>
        {renderPane("primary")}
        {secondaryTabs.length > 0 ? renderPane("secondary", !visibleSplit) : null}
      </div>
      <TransferManagerList />
      {closeTabIntent === null ? null : (
        <ConfirmDialog
          id="sftp-close-dirty-tab"
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: closeTabIntent.path })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={() => {
            const intent = closeTabIntent;
            setCloseTabIntent(null);
            closeTab(intent.pane, intent.id);
          }}
          onCancel={() => setCloseTabIntent(null)}
        />
      )}
      {visibleSplit && compareOpen ? (
        <SFTPCompareDialog
          left={{ alias: primaryLocation?.alias ?? "", path: primaryLocation?.path || "/" }}
          right={{ alias: secondaryLocation?.alias ?? "", path: secondaryLocation?.path || "/" }}
          onDismiss={() => setCompareOpen(false)}
        />
      ) : null}
    </section>
  );
}

import { useCallback, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { HostEntry } from "../api/config";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";
import { SFTPPanel, type SFTPTarget } from "./SFTPPanel";
import { TransferManagerList } from "./TransferManagerList";

const storageKey = "sshc.sftp.tabs";
const splitStorageKey = "sshc.sftp.split";
const secondaryStorageKey = "sshc.sftp.secondary";
const maxTabs = 8;

type SFTPTab = { id: string; alias: string; path: string };
type SFTPLocation = { alias: string; path: string };

function identifier(): string {
  return globalThis.crypto?.randomUUID?.() ?? `tab_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function blankTab(): SFTPTab {
  return { id: identifier(), alias: "", path: "" };
}

function restore(): SFTPTab[] {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(storageKey) ?? "[]");
    if (!Array.isArray(raw)) return [blankTab()];
    const tabs = raw.flatMap((value): SFTPTab[] => {
      if (typeof value !== "object" || value === null) return [];
      const tab = value as Record<string, unknown>;
      const alias = typeof tab.alias === "string" ? tab.alias : "";
      const path = typeof tab.path === "string" && tab.path.startsWith("/") ? tab.path : "";
      return [{ id: identifier(), alias, path }];
    }).slice(0, maxTabs);
    return tabs.length === 0 ? [blankTab()] : tabs;
  } catch {
    return [blankTab()];
  }
}

function remember(tabs: SFTPTab[]): void {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(tabs.map(({ alias, path }) => ({ alias, path }))));
  } catch {
    // A browser that refuses storage still keeps the tabs for this session.
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

function rememberSplit(split: boolean, location: SFTPLocation): void {
  try {
    window.localStorage.setItem(splitStorageKey, String(split));
    window.localStorage.setItem(secondaryStorageKey, JSON.stringify(location));
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

// Several remote directories open at once, each with its own host, history and
// selection. The transfer queue stays single: it is the engine's queue, not
// the tab's, so only the visible panel draws it.
export function SFTPWorkspace({
  aliases,
  hosts,
  target = null,
  onTargetHandled = () => undefined,
  onNavigationBlockerChange,
  onNavigateLocation,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  onTargetHandled?: (request: number) => void;
  onNavigationBlockerChange?: (blocker: NavigationBlocker | null) => void;
  onNavigateLocation?: (url: string) => void;
}) {
  const t = useTranslate();
  const [tabs, setTabs] = useState<SFTPTab[]>(restore);
  const [activeId, setActiveId] = useState(() => tabs[0]?.id ?? "");
  const [split, setSplit] = useState(restoreSplit);
  const [secondary, setSecondary] = useState<SFTPLocation>(restoreSecondary);
  // Restoring is a one-shot per tab: once a panel has opened its remembered
  // directory, later navigation inside it must not be pulled back.
  const restoring = useRef(new Map(tabs.map((tab) => [tab.id, { alias: tab.alias, path: tab.path }])));
  const secondaryRestoring = useRef(secondary);
  const blockers = useRef<{ primary: NavigationBlocker | null; secondary: NavigationBlocker | null }>({ primary: null, secondary: null });
  const active = tabs.some((tab) => tab.id === activeId) ? activeId : tabs[0]?.id ?? "";

  function commit(next: SFTPTab[]) {
    setTabs(next);
    remember(next);
  }

  function relocate(id: string, alias: string, path: string) {
    restoring.current.delete(id);
    setTabs((current) => {
      const next = current.map((tab) => tab.id === id ? { ...tab, alias, path } : tab);
      if (next.every((tab, index) => tab.alias === current[index]?.alias && tab.path === current[index]?.path)) {
        return current;
      }
      remember(next);
      return next;
    });
  }

  function addTab() {
    if (tabs.length >= maxTabs) return;
    const opened = blankTab();
    commit([...tabs, opened]);
    setActiveId(opened.id);
  }

  function closeTab(id: string) {
    const index = tabs.findIndex((tab) => tab.id === id);
    if (index < 0) return;
    restoring.current.delete(id);
    const remaining = tabs.filter((tab) => tab.id !== id);
    const next = remaining.length === 0 ? [blankTab()] : remaining;
    commit(next);
    if (id === active) setActiveId((next[Math.min(index, next.length - 1)] ?? next[0])?.id ?? "");
  }

  function switchTab(id: string) {
    setActiveId(id);
  }

  function moveWithKeyboard(event: ReactKeyboardEvent<HTMLButtonElement>, index: number) {
    activateTabFromKeyboard(event, index, tabs, (tab) => switchTab(tab.id));
  }

  function toggleSplit() {
    setSplit((current) => {
      const next = !current;
      rememberSplit(next, secondary);
      return next;
    });
  }

  function relocateSecondary(alias: string, path: string) {
    secondaryRestoring.current = { alias: "", path: "" };
    setSecondary((current) => {
      if (current.alias === alias && current.path === path) return current;
      const next = { alias, path };
      rememberSplit(split, next);
      return next;
    });
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

  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col" aria-label={t("sftp.tabs")}>
      <div className="flex min-h-12 shrink-0 items-stretch border-b border-line/60">
        <div role="tablist" aria-label={t("sftp.tabs")} className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
          {tabs.map((tab, index) => {
            const label = tabLabel(tab, t("sftp.newTab"));
            const selected = tab.id === active;
            return (
              <span
                key={tab.id}
                className={`group relative flex shrink-0 items-center border-b-2 ${selected ? "border-accent bg-select-fill/40" : "border-transparent hover:bg-toolbar/60"}`}
              >
                <button
                  type="button"
                  role="tab"
                  id={`sftp-tab-${tab.id}`}
                  aria-selected={selected}
                  aria-controls={`sftp-tabpanel-${tab.id}`}
                  tabIndex={selected ? 0 : -1}
                  onClick={() => switchTab(tab.id)}
                  onKeyDown={(event) => moveWithKeyboard(event, index)}
                  className={`min-h-12 max-w-64 truncate px-4 py-3 text-left text-sm ${selected ? "font-medium text-ink" : "text-ink-muted"}`}
                >
                  {label}
                </button>
                {tabs.length > 1 ? (
                  <button
                    type="button"
                    aria-label={t("sftp.closeTab", { name: label })}
                    onClick={() => closeTab(tab.id)}
                    className="flex size-12 items-center justify-center text-ink-faint hover:text-danger md:w-9"
                  >
                    <Icon name="close" className="size-3" />
                  </button>
                ) : null}
              </span>
            );
          })}
          <button
            type="button"
            aria-label={t("sftp.newTab")}
            disabled={tabs.length >= maxTabs}
            onClick={addTab}
            className="flex size-12 shrink-0 items-center justify-center text-ink-muted hover:bg-toolbar hover:text-ink disabled:text-ink-faint"
          >
            <Icon name="plus" className="size-4" />
          </button>
        </div>
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
      </div>

      <div className={`grid min-h-0 min-w-0 flex-1 gap-2 pt-2 ${split ? "lg:grid-cols-2" : "grid-cols-1"}`}>
        <div className="contents">
          {tabs.map((tab) => {
            const selected = tab.id === active;
            const restored = restoring.current.get(tab.id);
            return (
              <div
                key={tab.id}
                id={`sftp-tabpanel-${tab.id}`}
                role="tabpanel"
                aria-labelledby={`sftp-tab-${tab.id}`}
                hidden={!selected}
                className={selected ? "flex min-h-0 min-w-0 flex-col" : ""}
              >
                <SFTPPanel
                  aliases={aliases}
                  {...(hosts === undefined ? {} : { hosts })}
                  target={selected ? target : null}
                  initialLocation={restored === undefined || restored.alias === "" || restored.path === "" ? null : restored}
                  showTransfers={false}
                  onNavigationBlockerChange={selected ? updatePrimaryBlocker : undefined}
                  onNavigateLocation={onNavigateLocation}
                  onTargetHandled={onTargetHandled}
                  onLocationChange={(alias, path) => relocate(tab.id, alias, path)}
                />
              </div>
            );
          })}
        </div>
        <div hidden={!split} className={split ? "hidden min-h-0 min-w-0 flex-col lg:flex" : "hidden"} aria-label={t("sftp.secondPane")}>
          <SFTPPanel
            aliases={aliases}
            {...(hosts === undefined ? {} : { hosts })}
            initialLocation={secondaryRestoring.current.alias === "" || secondaryRestoring.current.path === "" ? null : secondaryRestoring.current}
            showTransfers={false}
            onNavigationBlockerChange={updateSecondaryBlocker}
            onNavigateLocation={onNavigateLocation}
            onLocationChange={relocateSecondary}
          />
        </div>
      </div>
      <TransferManagerList />
    </section>
  );
}

import type { SFTPSortState } from "./SFTPPanel";

// The SFTP workspace shows one or two panes side by side. Each pane owns an
// ordered list of tabs and remembers which one is selected. Every pane always
// has at least one tab: a pane whose last tab moves to the other pane
// disappears, while the last pane, or a pane just split, keeps a blank tab.
export const maxTabsPerPane = 8;
export const maxPanes = 2;

export type SFTPTab = { id: string; alias: string; path: string; sort: SFTPSortState };
export type SFTPPane = { id: string; tabs: SFTPTab[]; activeId: string };
export type PaneSide = "left" | "right";
// Where a tab is dropped: onto another pane, or beside the only pane to open a second one.
export type TabDestination = { paneId: string } | { side: PaneSide };

export function identifier(): string {
  return globalThis.crypto?.randomUUID?.() ?? `tab_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

export function blankTab(): SFTPTab {
  return { id: identifier(), alias: "", path: "", sort: { key: "name", direction: "ascending" } };
}

export function paneOf(tabs: SFTPTab[], activeId = tabs[0]?.id ?? ""): SFTPPane {
  return { id: identifier(), tabs, activeId };
}

export function blankPane(): SFTPPane {
  return paneOf([blankTab()]);
}

export function activeTab(pane: SFTPPane): SFTPTab {
  return pane.tabs.find((tab) => tab.id === pane.activeId) ?? pane.tabs[0] ?? blankTab();
}

export function findTab(panes: SFTPPane[], tabId: string): { pane: SFTPPane; tab: SFTPTab } | null {
  for (const pane of panes) {
    const tab = pane.tabs.find((candidate) => candidate.id === tabId);
    if (tab !== undefined) return { pane, tab };
  }
  return null;
}

export function allTabs(panes: SFTPPane[]): SFTPTab[] {
  return panes.flatMap((pane) => pane.tabs);
}

function replacePane(panes: SFTPPane[], next: SFTPPane): SFTPPane[] {
  return panes.map((pane) => pane.id === next.id ? next : pane);
}

function updateTab(panes: SFTPPane[], tabId: string, change: (tab: SFTPTab) => SFTPTab): SFTPPane[] {
  const found = findTab(panes, tabId);
  if (found === null) return panes;
  const changed = change(found.tab);
  if (changed === found.tab) return panes;
  return replacePane(panes, { ...found.pane, tabs: found.pane.tabs.map((tab) => tab.id === tabId ? changed : tab) });
}

// The pane without one tab. The selection moves to the neighbour that took
// the closed tab's place, as browsers do.
function withoutTab(pane: SFTPPane, tabId: string): SFTPPane {
  const index = pane.tabs.findIndex((tab) => tab.id === tabId);
  if (index < 0) return pane;
  const tabs = pane.tabs.filter((tab) => tab.id !== tabId);
  const activeId = pane.activeId === tabId ? tabs[Math.min(index, tabs.length - 1)]?.id ?? "" : pane.activeId;
  return { ...pane, tabs, activeId };
}

function withBlankTab(pane: SFTPPane): SFTPPane {
  const tab = blankTab();
  return { ...pane, tabs: [tab], activeId: tab.id };
}

function withoutEmptyPanes(panes: SFTPPane[]): SFTPPane[] {
  const remaining = panes.filter((pane) => pane.tabs.length > 0);
  if (remaining.length > 0) return remaining;
  return [withBlankTab(panes[0] ?? blankPane())];
}

export function addTab(panes: SFTPPane[], paneId: string, tab: SFTPTab): SFTPPane[] {
  const pane = panes.find((candidate) => candidate.id === paneId);
  if (pane === undefined || pane.tabs.length >= maxTabsPerPane) return panes;
  return replacePane(panes, { ...pane, tabs: [...pane.tabs, tab], activeId: tab.id });
}

export function closeTab(panes: SFTPPane[], tabId: string): SFTPPane[] {
  const found = findTab(panes, tabId);
  if (found === null) return panes;
  return withoutEmptyPanes(replacePane(panes, withoutTab(found.pane, tabId)));
}

export function selectTab(panes: SFTPPane[], tabId: string): SFTPPane[] {
  const found = findTab(panes, tabId);
  if (found === null || found.pane.activeId === tabId) return panes;
  return replacePane(panes, { ...found.pane, activeId: tabId });
}

export function relocateTab(panes: SFTPPane[], tabId: string, alias: string, path: string): SFTPPane[] {
  return updateTab(panes, tabId, (tab) => tab.alias === alias && tab.path === path ? tab : { ...tab, alias, path });
}

export function resortTab(panes: SFTPPane[], tabId: string, sort: SFTPSortState): SFTPPane[] {
  return updateTab(panes, tabId, (tab) => ({ ...tab, sort }));
}

export function canSplit(panes: SFTPPane[]): boolean {
  return panes.length < maxPanes;
}

export function moveTab(panes: SFTPPane[], tabId: string, destination: TabDestination): SFTPPane[] {
  const found = findTab(panes, tabId);
  if (found === null) return panes;
  if ("paneId" in destination) {
    const target = panes.find((pane) => pane.id === destination.paneId);
    if (target === undefined || target.id === found.pane.id || target.tabs.length >= maxTabsPerPane) return panes;
    return withoutEmptyPanes(panes.map((pane) => {
      if (pane.id === found.pane.id) return withoutTab(pane, tabId);
      if (pane.id === target.id) return { ...pane, tabs: [...pane.tabs, found.tab], activeId: found.tab.id };
      return pane;
    }));
  }
  if (!canSplit(panes)) return panes;
  // Splitting off the only tab leaves a blank tab behind, so one drag opens
  // the second pane and the first pane offers its host picker.
  const opened = paneOf([found.tab]);
  const emptied = withoutTab(found.pane, tabId);
  const remaining = replacePane(panes, emptied.tabs.length === 0 ? withBlankTab(emptied) : emptied);
  return destination.side === "left" ? [opened, ...remaining] : [...remaining, opened];
}

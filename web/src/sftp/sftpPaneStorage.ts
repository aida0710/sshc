import type { SFTPSort, SFTPSortState } from "./SFTPPanel";
import { isLocalPath, localHostAlias } from "./localHost";
import { blankPane, identifier, maxTabsPerPane, paneOf, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { clampSplitRatio } from "../ui/SplitResizeHandle";

// The workspace layout lives in localStorage under one key per pane slot. The
// left pane keeps the keys from the single-pane era so older layouts restore.
const leftTabsKey = "sshc.sftp.tabs";
const leftActiveKey = "sshc.sftp.activeTab";
const splitKey = "sshc.sftp.split";
const rightTabsKey = "sshc.sftp.secondaryTabs";
const rightActiveKey = "sshc.sftp.secondaryActiveTab";
const splitRatioKey = "sshc.sftp.splitRatio";

function restoredSort(value: Record<string, unknown>): SFTPSortState {
  const keys: readonly SFTPSort[] = ["name", "type", "size", "modified"];
  const key = typeof value.sortKey === "string" && keys.includes(value.sortKey as SFTPSort)
    ? value.sortKey as SFTPSort
    : "name";
  const direction = value.sortDirection === "descending" ? "descending" : "ascending";
  return { key, direction };
}

function restoreTabs(key: string): SFTPTab[] {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(key) ?? "[]");
    if (!Array.isArray(raw)) return [];
    return raw.flatMap((value): SFTPTab[] => {
      if (typeof value !== "object" || value === null) return [];
      const tab = value as Record<string, unknown>;
      const alias = typeof tab.alias === "string" ? tab.alias : "";
      const path = typeof tab.path === "string" &&
        (alias === localHostAlias ? isLocalPath(tab.path) : tab.path.startsWith("/")) ? tab.path : "";
      return [{ id: identifier(), alias, path, sort: restoredSort(tab) }];
    }).slice(0, maxTabsPerPane);
  } catch {
    return [];
  }
}

function restoreActiveId(key: string, tabs: SFTPTab[]): string {
  try {
    const index = Number.parseInt(window.localStorage.getItem(key) ?? "0", 10);
    return tabs[Number.isInteger(index) && index >= 0 && index < tabs.length ? index : 0]?.id ?? "";
  } catch {
    return tabs[0]?.id ?? "";
  }
}

function restoreSplit(): boolean {
  try {
    return window.localStorage.getItem(splitKey) === "true";
  } catch {
    return false;
  }
}

export function restorePanes(): SFTPPane[] {
  const leftTabs = restoreTabs(leftTabsKey);
  const left = leftTabs.length === 0 ? blankPane() : paneOf(leftTabs, restoreActiveId(leftActiveKey, leftTabs));
  if (!restoreSplit()) return [left];
  const rightTabs = restoreTabs(rightTabsKey);
  return rightTabs.length === 0 ? [left] : [left, paneOf(rightTabs, restoreActiveId(rightActiveKey, rightTabs))];
}

function storedTabs(pane: SFTPPane | undefined): string {
  return JSON.stringify((pane?.tabs ?? []).map(({ alias, path, sort }) => ({
    alias,
    path,
    sortKey: sort.key,
    sortDirection: sort.direction,
  })));
}

function storedActiveIndex(pane: SFTPPane | undefined): string {
  const index = pane?.tabs.findIndex((tab) => tab.id === pane.activeId) ?? -1;
  return String(index < 0 ? 0 : index);
}

export function rememberPanes(panes: SFTPPane[]): void {
  try {
    window.localStorage.setItem(leftTabsKey, storedTabs(panes[0]));
    window.localStorage.setItem(leftActiveKey, storedActiveIndex(panes[0]));
    window.localStorage.setItem(rightTabsKey, storedTabs(panes[1]));
    window.localStorage.setItem(rightActiveKey, storedActiveIndex(panes[1]));
    window.localStorage.setItem(splitKey, String(panes.length > 1));
  } catch {
    // A browser that refuses storage still keeps the panes for this session.
  }
}

export function restoreSplitRatio(): number {
  try {
    const stored = window.localStorage.getItem(splitRatioKey);
    return stored === null || stored.trim() === "" ? 50 : clampSplitRatio(Number(stored));
  } catch {
    return 50;
  }
}

export function rememberSplitRatio(ratio: number): void {
  try {
    window.localStorage.setItem(splitRatioKey, String(ratio));
  } catch {
    // Losing the preference is acceptable; the ratio still applies now.
  }
}

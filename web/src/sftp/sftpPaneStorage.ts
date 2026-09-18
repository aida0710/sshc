import type { SFTPSort, SFTPSortState } from "./SFTPPanel";
import { isLocalPath, localHostAlias } from "./localHost";
import { blankPane, identifier, maxTabsPerPane, paneOf, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { clampSplitRatio } from "../ui/SplitResizeHandle";
import { readStoredJSON, readStoredValue, writeStoredValue } from "../ui/browserStorage";

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
  const raw = readStoredJSON(key, []);
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((value): SFTPTab[] => {
    if (typeof value !== "object" || value === null) return [];
    const tab = value as Record<string, unknown>;
    const alias = typeof tab.alias === "string" ? tab.alias : "";
    const path = typeof tab.path === "string" &&
      (alias === localHostAlias ? isLocalPath(tab.path) : tab.path.startsWith("/")) ? tab.path : "";
    return [{ id: identifier(), alias, path, sort: restoredSort(tab) }];
  }).slice(0, maxTabsPerPane);
}

function restoreActiveId(key: string, tabs: SFTPTab[]): string {
  const index = Number.parseInt(readStoredValue(key) ?? "0", 10);
  return tabs[Number.isInteger(index) && index >= 0 && index < tabs.length ? index : 0]?.id ?? "";
}

function restoreSplit(): boolean {
  return readStoredValue(splitKey) === "true";
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
  writeStoredValue(leftTabsKey, storedTabs(panes[0]));
  writeStoredValue(leftActiveKey, storedActiveIndex(panes[0]));
  writeStoredValue(rightTabsKey, storedTabs(panes[1]));
  writeStoredValue(rightActiveKey, storedActiveIndex(panes[1]));
  writeStoredValue(splitKey, String(panes.length > 1));
}

export function restoreSplitRatio(): number {
  const stored = readStoredValue(splitRatioKey);
  return stored === null || stored.trim() === "" ? 50 : clampSplitRatio(Number(stored));
}

export function rememberSplitRatio(ratio: number): void {
  writeStoredValue(splitRatioKey, String(ratio));
}

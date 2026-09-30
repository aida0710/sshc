import type { SFTPSort, SFTPSortState } from "./SFTPPanel";
import { isLocalPath, localHostAlias } from "./localHost";
import { blankPane, maxPanes, maxTabsPerPane, paneOf, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { clampSplitRatio } from "../ui/SplitResizeHandle";
import { readStoredJSON, readStoredValue, writeStoredJSON, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { newIdentifier } from "../ui/randomIdentifier";

type StoredTab = { alias: string; path: string; sortKey: SFTPSort; sortDirection: SFTPSortState["direction"] };
type StoredPane = { tabs: StoredTab[]; activeIndex: number };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function restoredSort(value: Record<string, unknown>): SFTPSortState {
  const keys: readonly SFTPSort[] = ["name", "type", "size", "modified"];
  const key = typeof value.sortKey === "string" && keys.includes(value.sortKey as SFTPSort)
    ? value.sortKey as SFTPSort
    : "name";
  const direction = value.sortDirection === "descending" ? "descending" : "ascending";
  return { key, direction };
}

function restoreTab(value: unknown): SFTPTab[] {
  if (!isRecord(value)) return [];
  const alias = typeof value.alias === "string" ? value.alias : "";
  const path = typeof value.path === "string" &&
    (alias === localHostAlias ? isLocalPath(value.path) : value.path.startsWith("/")) ? value.path : "";
  return [{ id: newIdentifier(), alias, path, sort: restoredSort(value) }];
}

function restoreActiveId(value: unknown, tabs: SFTPTab[]): string {
  const index = typeof value === "number" && Number.isInteger(value) && value >= 0 && value < tabs.length ? value : 0;
  return tabs[index]?.id ?? "";
}

// A pane with no usable tab is left out, just as a pane disappears when its
// last tab leaves.
function restorePane(value: unknown): SFTPPane[] {
  if (!isRecord(value) || !Array.isArray(value.tabs)) return [];
  const tabs = value.tabs.flatMap(restoreTab).slice(0, maxTabsPerPane);
  return tabs.length === 0 ? [] : [paneOf(tabs, restoreActiveId(value.activeIndex, tabs))];
}

export function restorePanes(): SFTPPane[] {
  const stored = readStoredJSON(localStorageKeys.sftpPanes, []);
  const panes = Array.isArray(stored) ? stored.flatMap(restorePane).slice(0, maxPanes) : [];
  return panes.length === 0 ? [blankPane()] : panes;
}

function storedPane(pane: SFTPPane): StoredPane {
  const activeIndex = pane.tabs.findIndex((tab) => tab.id === pane.activeId);
  return {
    tabs: pane.tabs.map(({ alias, path, sort }) => ({ alias, path, sortKey: sort.key, sortDirection: sort.direction })),
    activeIndex: Math.max(activeIndex, 0),
  };
}

export function rememberPanes(panes: SFTPPane[]): void {
  writeStoredJSON(localStorageKeys.sftpPanes, panes.map(storedPane));
}

export function restoreSplitRatio(): number {
  const stored = readStoredValue(localStorageKeys.sftpSplitRatio);
  return stored === null || stored.trim() === "" ? 50 : clampSplitRatio(Number(stored));
}

export function rememberSplitRatio(ratio: number): void {
  writeStoredValue(localStorageKeys.sftpSplitRatio, String(ratio));
}

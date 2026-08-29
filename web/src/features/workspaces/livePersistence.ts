import { MAX_WORKSPACE_PANES, paneIDs, type LayoutState, type RuntimeNode } from "./layout";

export const liveWorkspaceStorageKey = "sshc.terminal.live-workspace.v1";

type StorageAccess = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export type LiveWorkspaceNode =
  | { pane: { id: string; sessionId: string }; split?: never }
  | {
      pane?: never;
      split: {
        direction: "horizontal" | "vertical";
        ratio: number;
        first: LiveWorkspaceNode;
        second: LiveWorkspaceNode;
      };
    };

export type LiveWorkspaceSnapshot = {
  root: LiveWorkspaceNode;
  focusedPaneId: string;
  focusModePaneId: string | null;
};

type PersistedLiveWorkspace = LiveWorkspaceSnapshot & { version: 1 };

export function browserSessionStorage(): StorageAccess | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

export function saveLiveWorkspace(
  storage: StorageAccess | null,
  layout: LayoutState | null,
  focusModePaneId: string | null,
): void {
  if (storage === null) return;
  try {
    const root = layout === null ? null : snapshotNode(layout.root);
    if (layout === null || root === null || paneIDs(layout.root).length < 2) {
      storage.removeItem(liveWorkspaceStorageKey);
      return;
    }
    const value: PersistedLiveWorkspace = {
      version: 1,
      root,
      focusedPaneId: layout.focusedPaneId,
      focusModePaneId,
    };
    storage.setItem(liveWorkspaceStorageKey, JSON.stringify(value));
  } catch {
    // Terminal operation must not depend on browser storage being available.
  }
}

export function loadLiveWorkspace(
  storage: StorageAccess | null,
  availableSessionIds: ReadonlySet<string>,
): LiveWorkspaceSnapshot | null {
  if (storage === null) return null;
  try {
    const raw = storage.getItem(liveWorkspaceStorageKey);
    if (raw === null || raw.length > 32_768) return discard(storage);
    const value = parseSnapshot(JSON.parse(raw));
    if (value === null) return discard(storage);
    const root = pruneNode(value.root, availableSessionIds);
    if (root === null || countPanes(root) < 2) return discard(storage);
    const paneIds = new Set(collectPaneIds(root));
    return {
      root,
      focusedPaneId: paneIds.has(value.focusedPaneId) ? value.focusedPaneId : collectPaneIds(root)[0] ?? "",
      focusModePaneId: value.focusModePaneId !== null && paneIds.has(value.focusModePaneId)
        ? value.focusModePaneId
        : null,
    };
  } catch {
    return discard(storage);
  }
}

function snapshotNode(root: RuntimeNode): LiveWorkspaceNode | null {
  if (root.pane !== undefined) {
    return root.pane.sessionId === undefined
      ? null
      : { pane: { id: root.pane.id, sessionId: root.pane.sessionId } };
  }
  const first = snapshotNode(root.split.first);
  const second = snapshotNode(root.split.second);
  if (first === null || second === null) return null;
  return { split: { direction: root.split.direction, ratio: root.split.ratio, first, second } };
}

function parseSnapshot(value: unknown): PersistedLiveWorkspace | null {
  if (!isRecord(value) || value.version !== 1 || !validIdentifier(value.focusedPaneId)) return null;
  if (value.focusModePaneId !== null && !validIdentifier(value.focusModePaneId)) return null;
  const seenPaneIds = new Set<string>();
  const seenSessionIds = new Set<string>();
  const root = parseNode(value.root, 0, seenPaneIds, seenSessionIds);
  if (root === null || seenPaneIds.size < 2 || seenPaneIds.size > MAX_WORKSPACE_PANES) return null;
  return {
    version: 1,
    root,
    focusedPaneId: value.focusedPaneId,
    focusModePaneId: value.focusModePaneId,
  };
}

function parseNode(
  value: unknown,
  depth: number,
  seenPaneIds: Set<string>,
  seenSessionIds: Set<string>,
): LiveWorkspaceNode | null {
  if (!isRecord(value) || depth >= MAX_WORKSPACE_PANES) return null;
  if (isRecord(value.pane)) {
    const id = value.pane.id;
    const sessionId = value.pane.sessionId;
    if (!validIdentifier(id) || !validIdentifier(sessionId) || seenPaneIds.has(id) || seenSessionIds.has(sessionId)) return null;
    seenPaneIds.add(id);
    seenSessionIds.add(sessionId);
    return { pane: { id, sessionId } };
  }
  if (!isRecord(value.split)) return null;
  const direction = value.split.direction;
  const ratio = value.split.ratio;
  if ((direction !== "horizontal" && direction !== "vertical") || typeof ratio !== "number" || !Number.isFinite(ratio) || ratio < 10 || ratio > 90) return null;
  const first = parseNode(value.split.first, depth + 1, seenPaneIds, seenSessionIds);
  const second = parseNode(value.split.second, depth + 1, seenPaneIds, seenSessionIds);
  return first === null || second === null ? null : { split: { direction, ratio, first, second } };
}

function pruneNode(root: LiveWorkspaceNode, available: ReadonlySet<string>): LiveWorkspaceNode | null {
  if (root.pane !== undefined) return available.has(root.pane.sessionId) ? root : null;
  const first = pruneNode(root.split.first, available);
  const second = pruneNode(root.split.second, available);
  if (first === null) return second;
  if (second === null) return first;
  return { split: { ...root.split, first, second } };
}

function collectPaneIds(root: LiveWorkspaceNode): string[] {
  if (root.pane !== undefined) return [root.pane.id];
  return [...collectPaneIds(root.split.first), ...collectPaneIds(root.split.second)];
}

function countPanes(root: LiveWorkspaceNode): number {
  return root.pane !== undefined ? 1 : countPanes(root.split.first) + countPanes(root.split.second);
}

function validIdentifier(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 256 && !/[\u0000-\u001f\u007f]/u.test(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function discard(storage: StorageAccess): null {
  try { storage.removeItem(liveWorkspaceStorageKey); } catch { /* ignore unavailable storage */ }
  return null;
}

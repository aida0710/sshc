import { useEffect, useMemo, useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { paneIds, type LayoutState } from "./layout";
import { browserSessionStorage, loadLiveWorkspace, saveLiveWorkspace } from "./livePersistence";
import { findPane, restoreLiveNode } from "./panes";
import type { WorkspaceRestoreRequest } from "./useWorkspaceRestore";

export type RestoredLiveWorkspace = {
  layout: LayoutState;
  focusModePaneId: string | null;
  name: string;
  focusedSessionId: string | undefined;
};

type LiveWorkspacePersistenceOptions = {
  sessionsLoaded: boolean;
  // A saved layout being reopened wins over what the tab kept.
  restoreRequest: WorkspaceRestoreRequest | null;
  sessionById: ReadonlyMap<string, TerminalSession>;
  layout: LayoutState | null;
  focusModePaneId: string | null;
  name: string;
  onRestored: (restored: RestoredLiveWorkspace) => void;
};

// useLiveWorkspacePersistence keeps the unsaved split in this tab's session
// storage, and after a reload rebuilds it from the terminals that are still
// open. Saving waits until that one restore has been tried, so the empty
// layout of the first render never overwrites what the tab kept.
// It returns whether the restore has been tried.
export function useLiveWorkspacePersistence({
  sessionsLoaded,
  restoreRequest,
  sessionById,
  layout,
  focusModePaneId,
  name,
  onRestored,
}: LiveWorkspacePersistenceOptions): boolean {
  const [ready, setReady] = useState(false);
  const storage = useMemo(() => browserSessionStorage(), []);

  useEffect(() => {
    if (!sessionsLoaded || ready) return;
    setReady(true);
    if (restoreRequest !== null) return;
    const restored = loadLiveWorkspace(storage, new Set(sessionById.keys()));
    if (restored === null) return;
    const root = restoreLiveNode(restored.root, sessionById);
    if (root === null) return;
    const restoredPaneIds = paneIds(root);
    if (restoredPaneIds.length < 2) return;
    const focusedPaneId = restoredPaneIds.includes(restored.focusedPaneId) ? restored.focusedPaneId : restoredPaneIds[0] ?? "";
    onRestored({
      layout: { root, focusedPaneId },
      focusModePaneId: restored.focusModePaneId,
      name: restored.name,
      focusedSessionId: findPane(root, focusedPaneId)?.sessionId,
    });
  }, [onRestored, ready, restoreRequest, sessionById, sessionsLoaded, storage]);

  useEffect(() => {
    if (!sessionsLoaded || !ready) return;
    saveLiveWorkspace(storage, layout, focusModePaneId, name);
  }, [focusModePaneId, layout, name, ready, sessionsLoaded, storage]);

  return ready;
}

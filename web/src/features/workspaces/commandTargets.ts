import type { TerminalSession } from "../../api/terminalSessions";
import { paneIDs, type LayoutState } from "./layout";
import { findPane } from "./panes";
import type { WorkspaceCommandTarget } from "./WorkspaceCommandCenter";

// Where a broadcast command can go: every pane of the visible workspace,
// or the single active terminal when no workspace is shown.
export function commandTargetsFor(
  visibleLayout: LayoutState | null,
  active: TerminalSession | null,
  sessionByID: ReadonlyMap<string, TerminalSession>,
): WorkspaceCommandTarget[] {
  if (visibleLayout !== null) return paneIDs(visibleLayout.root).map((id, index) => {
    const pane = findPane(visibleLayout.root, id);
    if (pane === null) throw new Error("workspace pane disappeared");
    const session = pane.sessionId === undefined ? undefined : sessionByID.get(pane.sessionId);
    const connected = session?.state === "connected";
    return {
      targetId: pane.id,
      ...(session === undefined ? {} : { sessionId: session.id }),
      alias: pane.alias,
      title: session?.title ?? pane.alias,
      paneNumber: index + 1,
      connected,
      state: session?.state ?? pane.state,
    };
  });
  if (active === null) return [];
  return [{
    targetId: active.id,
    sessionId: active.id,
    alias: active.kind === "ssh" ? active.alias ?? active.title : "localhost",
    title: active.title,
    paneNumber: 1,
    connected: active.state === "connected",
    state: active.state,
  }];
}

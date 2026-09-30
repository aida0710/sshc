import type { TerminalSession } from "../../api/terminalSessions";
import { terminalSubtitle } from "../../terminal/terminalPresentation";
import {
  paneIds,
  reduceLayout,
  restoreLayout,
  storeLayout,
  type DockEdge,
  type LayoutState,
  type RuntimeNode,
  type RuntimePane,
  type StoredNode,
  type StoredPane,
} from "./layout";
import type { LiveWorkspaceNode } from "./livePersistence";
import { newIdentifier } from "../../ui/randomIdentifier";

export function visit(root: StoredNode, callback: (pane: StoredPane) => void) {
  if (root.pane !== undefined) { callback(root.pane); return; }
  visit(root.split.first, callback); visit(root.split.second, callback);
}

export function paneForSession(session: TerminalSession, id = newIdentifier()): RuntimePane {
  return {
    id,
    alias: terminalSubtitle(session),
    ...(session.kind === "shell" ? { kind: "shell" as const } : {}),
    state: session.state === "connected" ? "connected" : session.state === "exited" ? "failed" : "connecting",
    sessionId: session.id,
    ...(session.problem === "" ? {} : { problem: session.problem }),
  };
}

// A layout holding just this session, the seed for docking another one
// beside it or for saving a single terminal as a workspace.
export function singlePaneLayout(session: TerminalSession, id = newIdentifier()): LayoutState {
  const pane = paneForSession(session, id);
  const seeded = restoreLayout({ pane: {
    id: pane.id,
    alias: pane.alias,
    ...(pane.kind === undefined ? {} : { kind: pane.kind }),
  } }, id);
  return reduceLayout(seeded, { type: "connection-started", paneId: id, sessionId: session.id });
}

// Docking a session on the edge of a terminal shown on its own makes the two
// a new split.
export function dockedPairLayout(target: TerminalSession, source: TerminalSession, edge: DockEdge): LayoutState {
  const targetPaneId = newIdentifier();
  return reduceLayout(singlePaneLayout(target, targetPaneId), {
    type: "dock-pane",
    targetPaneId,
    edge,
    pane: paneForSession(source),
  });
}

// Closes the panes of the given closed sessions. A split left with one pane is
// no split, so that returns null.
export function withoutClosedSessions(layout: LayoutState, closedSessionIds: ReadonlySet<string>): LayoutState | null {
  let next = layout;
  for (const paneId of paneIds(layout.root)) {
    const pane = findPane(next.root, paneId);
    if (pane?.sessionId !== undefined && closedSessionIds.has(pane.sessionId)) {
      next = reduceLayout(next, { type: "close", paneId });
    }
  }
  return paneIds(next.root).length < 2 ? null : next;
}

export function findPane(root: RuntimeNode, id: string): RuntimePane | null {
  if (root.pane !== undefined) return root.pane.id === id ? root.pane : null;
  return findPane(root.split.first, id) ?? findPane(root.split.second, id);
}

export function findPaneBySession(root: RuntimeNode, sessionId: string): RuntimePane | null {
  if (root.pane !== undefined) return root.pane.sessionId === sessionId ? root.pane : null;
  return findPaneBySession(root.split.first, sessionId) ?? findPaneBySession(root.split.second, sessionId);
}

// Rebuilds a stored live layout from the sessions that still exist; panes
// whose session is gone drop out and their sibling takes the whole split.
export function restoreLiveNode(root: LiveWorkspaceNode, sessions: ReadonlyMap<string, TerminalSession>): RuntimeNode | null {
  if (root.pane !== undefined) {
    const session = sessions.get(root.pane.sessionId);
    return session === undefined ? null : { pane: paneForSession(session, root.pane.id) };
  }
  const first = restoreLiveNode(root.split.first, sessions);
  const second = restoreLiveNode(root.split.second, sessions);
  if (first === null) return second;
  if (second === null) return first;
  return { split: { ...root.split, first, second } };
}

// The name a workspace gets from its hosts when nobody named it:
// "web + db", or "web + db +2" past two distinct hosts.
export function automaticWorkspaceName(layout: LayoutState): string {
  const aliases: string[] = [];
  visit(storeLayout(layout).layout, (pane) => aliases.push(pane.alias));
  const uniqueAliases = [...new Set(aliases)];
  return uniqueAliases.slice(0, 2).join(" + ") + (uniqueAliases.length > 2 ? ` +${uniqueAliases.length - 2}` : "");
}

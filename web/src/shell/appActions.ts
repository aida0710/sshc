import type { TerminalSession } from "../api/terminalSessions";
import type { ShortcutAction } from "../keyconfig/bindings";
import type { Section } from "../routing/sectionRoute";
import type { PaneEdge } from "../terminal/splitView/layout";

// SessionShortcut is what the new session and close session shortcuts ask of the session list
// (WorkspaceTree): its picker for a new session, or its confirmation before closing one.
export type SessionShortcut = { action: "newSession" } | { action: "closeSession"; sessionId: string };

export type AppActionContext = {
  // terminalFace is whether the terminal screen is the one shown.
  terminalFace: boolean;
  orderedSessions: readonly TerminalSession[];
  activeSessionId: string | null;
  // panes are the sessions of the split shown on the terminal screen, in order; one or none outside a split.
  panes: readonly string[];
  // splitBesideActive asks what goes beside the session shown, or is null when nothing can.
  splitBesideActive: ((edge: PaneEdge) => void) | null;
  navigate: (section: Section) => void;
  showSession: (id: string) => void;
  openPalette: () => void;
  requestSession: (request: SessionShortcut) => void;
  toggleNavigation: () => void;
};

// cycled is the item delta places away from current in items, going round at either end. Without a current
// one, forwards starts at the first and backwards at the last.
function cycled(items: readonly string[], current: string | null, delta: number): string | undefined {
  const index = current === null ? -1 : items.indexOf(current);
  if (index < 0) return items[delta > 0 ? 0 : items.length - 1];
  return items[(index + delta + items.length) % items.length];
}

// appActions is what the shortcut actions of the whole app do, as things stand: the ones that cannot act now are
// left out. A shortcut and the command palette both run these.
export function appActions({ terminalFace, orderedSessions, activeSessionId, panes, splitBesideActive, navigate, showSession, openPalette, requestSession, toggleNavigation }: AppActionContext): Partial<Record<ShortcutAction, () => void>> {
  const showNeighbour = (items: readonly string[], delta: number) => () => {
    const neighbour = cycled(items, activeSessionId, delta);
    if (neighbour !== undefined) showSession(neighbour);
  };
  const sessionIds = orderedSessions.map((session) => session.id);
  return {
    palette: openPalette,
    home: () => navigate("Home"),
    sftp: () => navigate("Files"),
    toggleNavigation,
    newSession: () => requestSession({ action: "newSession" }),
    ...(terminalFace && activeSessionId !== null ? { closeSession: () => requestSession({ action: "closeSession", sessionId: activeSessionId }) } : {}),
    ...(terminalFace && splitBesideActive !== null ? { splitRight: () => splitBesideActive("right"), splitBelow: () => splitBesideActive("bottom") } : {}),
    ...(terminalFace && panes.length > 1 ? { nextPane: showNeighbour(panes, 1), previousPane: showNeighbour(panes, -1) } : {}),
    ...(sessionIds.length > 0 ? { nextSession: showNeighbour(sessionIds, 1), previousSession: showNeighbour(sessionIds, -1) } : {}),
  };
}

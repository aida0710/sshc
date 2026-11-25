import type { TerminalSession } from "../api/terminalSessions";
import type { ShortcutAction } from "../keyconfig/bindings";
import type { Section } from "../routing/sectionRoute";
import type { PaneEdge } from "../terminal/splitView/layout";
import { runTargetNumberActions } from "./runTargetChoice";

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
  // runTargets はリモートセッションの一覧の並び（runTargetChoices）、selectedRunTarget はそのうち選んでいるもの。
  runTargets: readonly (string | null)[];
  selectedRunTarget: string | null;
  // chooseRunTarget は、一覧の行を押したときと同じことをする（runTargetChoice.ts の chooseRunTarget）。
  chooseRunTarget: (alias: string | null) => void;
};

// cycled is the item delta places away from the one at currentIndex in items, going round at either end. Without a
// current one (currentIndex -1), forwards starts at the first and backwards at the last.
function cycled<T>(items: readonly T[], currentIndex: number, delta: number): T | undefined {
  if (currentIndex < 0) return items[delta > 0 ? 0 : items.length - 1];
  return items[(currentIndex + delta + items.length) % items.length];
}

// runTargetActions は、リモートセッションを切り替える操作。このマシンしか無い間は切り替える先が無いので、どれも返さない。
// 番号の操作は、その番号のリモートセッションがあるものだけを返す。返さなかった番号のキー（Ctrl+3 など）は、
// ターミナルのプログラムへそのまま届く。
function runTargetActions({ runTargets, selectedRunTarget, chooseRunTarget }: AppActionContext): Partial<Record<ShortcutAction, () => void>> {
  if (runTargets.length < 2) return {};
  const chooseBeside = (delta: number) => () => {
    const beside = cycled(runTargets, runTargets.indexOf(selectedRunTarget), delta);
    if (beside !== undefined) chooseRunTarget(beside);
  };
  const numbered = runTargetNumberActions.flatMap((action, index) => {
    const alias = runTargets[index];
    return alias === undefined ? [] : [[action, () => chooseRunTarget(alias)] as const];
  });
  return { nextRunTarget: chooseBeside(1), previousRunTarget: chooseBeside(-1), ...Object.fromEntries(numbered) };
}

// appActions is what the shortcut actions of the whole app do, as things stand: the ones that cannot act now are
// left out. A shortcut and the command palette both run these.
export function appActions(context: AppActionContext): Partial<Record<ShortcutAction, () => void>> {
  const { terminalFace, orderedSessions, activeSessionId, panes, splitBesideActive, navigate, showSession, openPalette, requestSession, toggleNavigation } = context;
  const showNeighbour = (items: readonly string[], delta: number) => () => {
    const neighbour = cycled(items, activeSessionId === null ? -1 : items.indexOf(activeSessionId), delta);
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
    ...runTargetActions(context),
  };
}

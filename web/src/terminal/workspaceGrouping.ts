import type { TerminalSession, TerminalSessionPlace } from "../api/terminalSessions";
import { agentsWorkspaceId, defaultWorkspaceId, type TerminalWorkspace } from "../api/terminalWorkspaces";
import type { Translate } from "../i18n/context";
import { interfaceState, type SettingsDocument } from "../settings/settingsStore";

export type WorkspaceGroup = {
  workspace: TerminalWorkspace;
  sessions: TerminalSession[];
};

// The sidebar tree: every workspace in the engine's order with its sessions in
// the engine's list order. Hidden workspaces are left out unless asked for. A session whose
// workspace is not listed yet (another tab just made it) shows under default
// until the list catches up, rather than disappearing.
export function groupSessionsByWorkspace(
  workspaces: readonly TerminalWorkspace[],
  sessions: readonly TerminalSession[],
  showHidden: boolean,
): WorkspaceGroup[] {
  const known = new Set(workspaces.map((workspace) => workspace.id));
  const groupOf = (session: TerminalSession) => known.has(session.workspaceId) ? session.workspaceId : defaultWorkspaceId;
  return workspaces
    .filter((workspace) => showHidden || !workspace.hidden)
    .map((workspace) => ({ workspace, sessions: sessions.filter((session) => groupOf(session) === workspace.id) }));
}

// showsHiddenWorkspaces は、サイドバーの木が隠したワークスペースも出すかである。このマシンの画面の状態
// （useThisMachineSettings）に残る。
export function showsHiddenWorkspaces(thisMachineSettings: SettingsDocument | null): boolean {
  return interfaceState(thisMachineSettings, "showHiddenTerminalWorkspaces") === "true";
}

// The session at the top of the sidebar: the first of the first workspace that
// has any. Hidden workspaces are passed over even while the tree shows them, so
// what an agent opened is not chosen for the person. Before the workspaces are
// read the tree has no top, and this is null.
export function topSessionId(workspaces: readonly TerminalWorkspace[], sessions: readonly TerminalSession[]): string | null {
  return groupSessionsByWorkspace(workspaces, sessions, false).flatMap((group) => group.sessions)[0]?.id ?? null;
}

// A built-in workspace with no name of its own is shown by the name this
// language gives it.
export function workspaceDisplayName(t: Translate, workspace: TerminalWorkspace): string {
  if (workspace.name !== "") return workspace.name;
  if (workspace.id === defaultWorkspaceId) return t("workspace.defaultName");
  if (workspace.id === agentsWorkspaceId) return t("workspace.agentsName");
  return workspace.id;
}

// Where a workspace is moved to: right before or right after the target workspace.
export type WorkspacePlaceSide = "before" | "after";
export type WorkspacePlace = { targetId: string; side: WorkspacePlaceSide };

// The engine puts a moved workspace before another one, or at the end (null). Right after target is
// before whatever follows target in the engine's whole list, hidden workspaces included, so the moved
// workspace lands next to target even when a hidden one comes after it.
export function beforeIdForWorkspaceMove(allWorkspaceIds: readonly string[], workspaceId: string, place: WorkspacePlace): string | null {
  if (place.side === "before") return place.targetId;
  const rest = allWorkspaceIds.filter((id) => id !== workspaceId);
  return rest[rest.indexOf(place.targetId) + 1] ?? null;
}

// sessionsAfterMove は、エンジンが id のセッションを place へ移したあとの一覧である。エンジンと同じく、
// beforeId のセッションの前に、null なら一覧の最後（そのワークスペースの最後）に置く。エンジンの答えが
// 届くまでの間だけ使い、答えが届いたらそちらに置き換わる。どちらのセッションも無ければ、元のままにする。
export function sessionsAfterMove(sessions: readonly TerminalSession[], id: string, place: TerminalSessionPlace): TerminalSession[] {
  const moved = sessions.find((session) => session.id === id);
  if (moved === undefined) return [...sessions];
  const placed = { ...moved, workspaceId: place.workspaceId };
  if (place.beforeId === id) return sessions.map((session) => session.id === id ? placed : session);
  const rest = sessions.filter((session) => session.id !== id);
  const at = place.beforeId === null ? rest.length : rest.findIndex((session) => session.id === place.beforeId);
  if (at < 0) return [...sessions];
  rest.splice(at, 0, placed);
  return rest;
}

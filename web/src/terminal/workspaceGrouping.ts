import type { TerminalSession } from "../api/terminalSessions";
import { agentsWorkspaceId, defaultWorkspaceId, type TerminalWorkspace } from "../api/terminalWorkspaces";
import type { Translate } from "../i18n/context";

export type WorkspaceGroup = {
  workspace: TerminalWorkspace;
  sessions: TerminalSession[];
};

// The sidebar tree: every workspace in the engine's order with its sessions in
// list order. Hidden workspaces are left out unless asked for. A session whose
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

// A built-in workspace with no name of its own is shown by the name this
// language gives it.
export function workspaceDisplayName(t: Translate, workspace: TerminalWorkspace): string {
  if (workspace.name !== "") return workspace.name;
  if (workspace.id === defaultWorkspaceId) return t("workspace.defaultName");
  if (workspace.id === agentsWorkspaceId) return t("workspace.agentsName");
  return workspace.id;
}

// Reordering inside one workspace keeps every other session where it was: the
// group's sessions take the group's old positions in the new order.
export function orderWithGroupReordered(allSessionIds: readonly string[], reorderedGroup: readonly string[]): string[] {
  const members = new Set(reorderedGroup);
  const next = [...reorderedGroup];
  return allSessionIds.map((id) => members.has(id) ? next.shift() ?? id : id);
}

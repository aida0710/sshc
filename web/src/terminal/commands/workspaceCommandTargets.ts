import type { TerminalSession } from "../../api/terminalSessions";
import { terminalSubtitle } from "../terminalPresentation";
import type { WorkspaceCommandTarget } from "./WorkspaceCommandCenter";

// Where a command sent to a workspace goes: each of its sessions, numbered in
// list order. Only the connected ones receive it; the rest are shown as skipped.
export function commandTargetsForWorkspace(sessions: readonly TerminalSession[]): WorkspaceCommandTarget[] {
  return sessions.map((session, index) => ({
    targetId: session.id,
    sessionId: session.id,
    alias: terminalSubtitle(session),
    title: session.title,
    paneNumber: index + 1,
    connected: session.state === "connected",
    state: session.state,
  }));
}

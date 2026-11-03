import type { OpenTerminalSessionRequest, TerminalSession } from "../api/terminalSessions";
import { sshSessionTarget } from "../connections/directDestination";

// duplicateSessionRequest opens what session runs again: the same connection or shell, in the same workspace and
// in the folder the original works in, when the engine knows it.
export function duplicateSessionRequest(session: TerminalSession): OpenTerminalSessionRequest {
  const startIn = session.workingDirectory === undefined ? {} : { cwd: session.workingDirectory };
  return session.kind === "ssh" && session.alias !== undefined
    ? { kind: "ssh", ...sshSessionTarget(session.alias), workspaceId: session.workspaceId, ...startIn }
    : {
        kind: "shell", workspaceId: session.workspaceId, ...startIn,
        ...(session.shellProfileId === undefined ? {} : { profileId: session.shellProfileId }),
      };
}

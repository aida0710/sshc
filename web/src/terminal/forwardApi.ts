import { engineStreamPath, engineTerminalBase } from "../api/remoteEngines";
import { createTerminalSessionsApi, terminalSessionsApi, type TerminalSessionsApi } from "../api/terminalSessions";

// forwardApiFor is the port forwarding API of the sessions on runTarget, or on this machine. For a run
// target, this machine's engine listens and carries the connections to the run target (see
// internal/httpserver/remote_terminal_forwards.go).
export function forwardApiFor(runTarget: string | undefined): Pick<TerminalSessionsApi, "startTerminalForward" | "stopTerminalForward"> {
  return runTarget === undefined ? terminalSessionsApi : createTerminalSessionsApi(engineTerminalBase(runTarget), engineStreamPath(runTarget));
}

import type { TerminalSession } from "../api/terminalSessions";
import { localHostAlias } from "../sftp/localHost";
import { savedConnectionOf } from "./terminalPresentation";

// pathPreviewAlias names the machine whose files a session's shell sees, as the engine that runs the session reads
// them. A path a program prints is a path there: a shell runs on the engine's own machine, and an SSH session on its
// alias. The files are read with that engine's API (EngineApis), so a session of a remote session's engine is read
// by that engine, with its own SSH configuration.
export function pathPreviewAlias(session: Pick<TerminalSession, "kind" | "alias">): string | undefined {
  return session.kind === "shell" ? localHostAlias : savedConnectionOf(session);
}

import type { TerminalSession } from "../api/terminalSessions";
import { localHostAlias } from "../sftp/localHost";

// pathPreviewAlias names the machine whose files a session's shell sees, as an
// alias this PC can read files from. A path a program prints is a path there:
// a shell runs on this PC, or on the run target it was opened on, and an SSH
// session runs on its alias. An SSH session opened on a run target reached its
// host through that engine's SSH config, which this PC may not share, so it has
// no alias here.
export function pathPreviewAlias(session: Pick<TerminalSession, "kind" | "alias">, runTarget: string | undefined): string | undefined {
  if (session.kind === "shell") return runTarget ?? localHostAlias;
  if (runTarget !== undefined) return undefined;
  return session.alias;
}

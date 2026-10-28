import type { TerminalSession } from "../api/terminalSessions";
import { localHostAlias } from "../sftp/localHost";

// pathPreviewAlias names the machine whose files a session's shell sees, as an
// alias this machine can read files from. A path a program prints is a path there:
// a shell runs on this machine, or on the run target it was opened on, and an SSH
// session runs on its alias. An SSH session opened on a run target reached its
// host through that engine's SSH config, which this machine may not share, so it
// has an alias here only when the alias reaches the same host from both
// (sameDestination, from useSameDestination).
export function pathPreviewAlias(
  session: Pick<TerminalSession, "kind" | "alias">,
  runTarget: string | undefined,
  sameDestination: boolean,
): string | undefined {
  if (session.kind === "shell") return runTarget ?? localHostAlias;
  if (runTarget !== undefined && !sameDestination) return undefined;
  return session.alias;
}

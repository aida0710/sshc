import type { TerminalSession } from "../api/terminalSessions";
import { isDirectDestination } from "../connections/directDestination";
import type { Translate } from "../i18n/context";

export function terminalDisplayTitle(session: TerminalSession): string {
  return session.presentation?.displayTitle ?? session.title;
}

// Where the session talks to: the alias for ssh, the machine itself for a
// local shell. Screens that have a translator show "localhost" in the
// person's language; layout code keeps the bare word as the pane's alias.
export function terminalSubtitle(session: TerminalSession, translate?: Translate): string {
  if (session.kind === "ssh") return session.alias ?? session.title;
  return translate === undefined ? "localhost" : translate("terminal.localhost");
}

// savedConnectionOf is the connection name an SSH session was opened with. A session opened on a user@host[:port] has
// none, so nothing can be read or saved through a connection for it (SFTP, a port forward kept in its Host block).
export function savedConnectionOf(session: Pick<TerminalSession, "kind" | "alias">): string | undefined {
  if (session.kind !== "ssh" || session.alias === undefined || isDirectDestination(session.alias)) return undefined;
  return session.alias;
}

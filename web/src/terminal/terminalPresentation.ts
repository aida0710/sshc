import type { TerminalSession } from "../api/terminalSessions";
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

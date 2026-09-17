import type { TerminalSession } from "../api/terminalSessions";

export function terminalDisplayTitle(session: TerminalSession): string {
  return session.presentation?.displayTitle ?? session.title;
}

export function terminalSubtitle(session: TerminalSession): string {
  if (session.kind === "ssh") return session.alias ?? session.title;
  return "localhost";
}

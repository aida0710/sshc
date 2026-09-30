import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { terminalDisplayTitle } from "./terminalPresentation";

// A program inside the terminal asked for a desktop notification through the
// standard OSC 9, OSC 99 or OSC 777 sequences. The engine records it on the
// session; the browser decides how to surface it.
export type TerminalNotification = {
  title: string;
  body: string;
};

export type UnreadSessions = ReadonlySet<string>;

// nextTerminalNotification returns the notification to surface for a session
// whose notificationVersion moved past what this browser had already seen.
// The first observation of a session is history, never a new event.
export function nextTerminalNotification(
  t: Translate,
  session: TerminalSession,
  previousVersion: number | undefined,
): TerminalNotification | null {
  if (previousVersion === undefined || (session.notificationVersion ?? 0) <= previousVersion) return null;
  const last = session.lastNotification;
  if (last === undefined) return null;
  const displayTitle = terminalDisplayTitle(session);
  const alias = session.kind === "ssh" ? session.alias : undefined;
  const subject = alias === undefined || alias === displayTitle ? displayTitle : `${displayTitle}（${alias}）`;
  // The browser notification names the pane so several agents stay apart;
  // the program's own title becomes the first line of the body.
  const body = last.title !== "" && last.body !== ""
    ? `${last.title}\n${last.body}`
    : last.title || last.body || t("terminal.notificationFallback");
  return { title: subject, body };
}

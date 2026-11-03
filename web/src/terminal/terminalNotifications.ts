import type { components } from "../api/schema";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { programStatusText } from "./programStatus";
import { terminalDisplayTitle } from "./terminalPresentation";

// What the engine recorded on the session: a program inside the terminal asked
// for a desktop notification through OSC 9, OSC 99 or OSC 777, or a long
// command the shell reported through OSC 133 finished. The browser decides how
// to surface it.
type RecordedNotification = components["schemas"]["TerminalNotification"];

// TerminalNotification is what a desktop notification shows.
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
  return { title: notificationSubject(session), body: notificationText(t, last) };
}

// blockedProgramNotification is what a notification says about a program waiting
// for the person (OSC 7501), or null when the session's program does not wait.
export function blockedProgramNotification(t: Translate, session: TerminalSession): TerminalNotification | null {
  const status = session.programStatus;
  if (status?.state !== "blocked") return null;
  return { title: notificationSubject(session), body: programStatusText(t, { ...status, state: "blocked" }) };
}

// The connection alias leads the heading because it is the only part the
// server cannot choose: the pane title follows the program's OSC title, so a
// hostile server could otherwise word the whole notification like sshc's own.
// The pane title still follows so several agents on one host stay apart, and
// the program's own title becomes the first line of the body.
function notificationSubject(session: TerminalSession): string {
  const displayTitle = terminalDisplayTitle(session);
  const alias = session.kind === "ssh" ? session.alias : undefined;
  return alias === undefined || alias === displayTitle ? displayTitle : `${alias}（${displayTitle}）`;
}

// notificationText is what a terminal notification says: how a long command
// ended, or the program's title and body on two lines, whichever it sent, or a
// fallback when it sent neither. The desktop notification and the sidebar's
// unread line both show it.
export function notificationText(t: Translate, notification: RecordedNotification): string {
  const command = notification.finishedCommand;
  if (command !== undefined) {
    const seconds = String(command.seconds);
    // A shell that reports no exit code is not taken to have failed.
    return command.exitCode === undefined || command.exitCode === 0
      ? t("terminal.longCommandCompleted", { seconds })
      : t("terminal.longCommandFailed", { seconds, code: String(command.exitCode) });
  }
  return notification.title !== "" && notification.body !== ""
    ? `${notification.title}\n${notification.body}`
    : notification.title || notification.body || t("terminal.notificationFallback");
}

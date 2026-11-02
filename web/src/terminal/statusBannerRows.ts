import type { TerminalSession } from "../api/terminalSessions";
import { exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";

// StatusBannerRow is one row of the strip of notices about a session (TerminalStatusBanners).
export type StatusBannerRow =
  // A message from the view itself, such as a refused clipboard write.
  | "view-problem"
  | "reconnect-failed"
  | "host-key"
  | "reconnecting"
  // Why the connection is in trouble while the session still runs.
  | "connection-problem"
  | "link"
  | "ended";

// NoticeSeverity is how a phone's notices button is coloured: danger once something ended or failed, notice while the
// engine is working on it or a question waits.
export type NoticeSeverity = "danger" | "notice";

type SessionNotices = {
  session: TerminalSession;
  problem: string;
  link: StreamLink;
  reconnectFailed: boolean;
  // answersHostKey is whether the view can answer the host key question from a button.
  answersHostKey: boolean;
};

// statusBannerRows is which rows the strip shows for a session, in order. A row only appears while it has something to
// say, and one thing is said once: an ended session's reason (why reconnecting stopped, why it could not connect) is its
// ended row, not a row of its own above it.
export function statusBannerRows({ session, problem, link, reconnectFailed, answersHostKey }: SessionNotices): StatusBannerRow[] {
  const rows: StatusBannerRow[] = [];
  if (problem !== "") rows.push("view-problem");
  if (reconnectFailed) rows.push("reconnect-failed");
  if (hostKeyQuestionOf(session) !== undefined && answersHostKey) rows.push("host-key");
  if (session.state === "reconnecting") rows.push("reconnecting");
  if (session.problem !== "" && session.exited === undefined) rows.push("connection-problem");
  if (link.phase !== "live") rows.push("link");
  if (session.exited !== undefined) rows.push("ended");
  return rows;
}

// hostKeyQuestionOf is the host key question a connecting session waits on, if any.
export function hostKeyQuestionOf(session: TerminalSession): TerminalSession["hostKeyQuestion"] {
  return session.state === "connecting" || session.state === "reconnecting" ? session.hostKeyQuestion : undefined;
}

// noticeSeverity is the severity of the rows shown, or null when there are none.
export function noticeSeverity(rows: readonly StatusBannerRow[], { session, link }: Pick<SessionNotices, "session" | "link">): NoticeSeverity | null {
  if (rows.length === 0) return null;
  const failed = rows.some((row) =>
    row === "reconnect-failed" || row === "connection-problem" ||
    (row === "link" && link.phase === "stopped") ||
    (row === "ended" && (session.problem !== "" || exitedAbnormally(session))));
  return failed ? "danger" : "notice";
}

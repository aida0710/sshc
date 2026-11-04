import type { TerminalSession } from "../api/terminalSessions";
import { exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import type { TerminalNotice } from "./terminalNotice";

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
  | "ended"
  // A program that ran in the lost shell can be started again in the new one.
  | "resume-program";

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
  if (session.resume !== undefined) rows.push("resume-program");
  return rows;
}

// rowsSaidByCard is which rows of the strip say the same as the card of a notice (TerminalNoticeCard), with the same
// action. The reason an ended session could not reconnect is the body of its card.
export function rowsSaidByCard(kind: TerminalNotice["kind"]): readonly StatusBannerRow[] {
  switch (kind) {
    case "host-key":
      return ["host-key"];
    case "reconnecting":
      return ["reconnecting"];
    case "ended":
      return ["ended", "reconnect-failed"];
    case "link-stopped":
      return ["link"];
    case "resume-program":
      return ["resume-program"];
    case "password-save":
    case "notification-permission":
      return [];
  }
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

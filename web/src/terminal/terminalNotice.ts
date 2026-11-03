import type { TerminalSession } from "../api/terminalSessions";
import { exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";

// TerminalNotice is a state of a session shown in a card over the terminal as well as in the strip above it, for a
// state the person is likely to act on. key names the occurrence, so a card hidden by the person stays hidden until
// the state happens again.
export type TerminalNotice =
  | { kind: "host-key"; key: string }
  | { kind: "ended"; key: string }
  | { kind: "reconnecting"; key: string }
  | { kind: "link-stopped"; key: string }
  | { kind: "password-save"; key: string }
  | { kind: "notification-permission"; key: string };

// terminalNoticeFor picks the card for a session, if any. A clean exit (the person typed exit) and a link to the engine
// that is still retrying by itself stay in the strip alone. asksNotificationPermission (the app has a reason to ask now,
// see useNotificationPermissionNotice) comes last: what is wrong with the session goes first.
export function terminalNoticeFor(session: TerminalSession, link: StreamLink, asksNotificationPermission = false): TerminalNotice | null {
  if (session.state === "exited" && session.exited !== undefined) {
    if (session.problem === "" && !exitedAbnormally(session)) return null;
    // Not exited.at: the stream reports the end before the list says when, and a new key would be a new card, which
    // takes the focus again. Reconnecting by hand starts the session anew, so its next end is a new occurrence.
    return { kind: "ended", key: `${noticeKeyPrefix(session.id)}ended:${session.startedAt}` };
  }
  const question = session.hostKeyQuestion;
  if (question !== undefined && (session.state === "connecting" || session.state === "reconnecting")) {
    return { kind: "host-key", key: `${noticeKeyPrefix(session.id)}host-key:${question.fingerprint}` };
  }
  if (session.state === "reconnecting") return { kind: "reconnecting", key: `${noticeKeyPrefix(session.id)}reconnecting` };
  if (link.phase === "stopped" && !link.gone) return { kind: "link-stopped", key: `${noticeKeyPrefix(session.id)}link-stopped` };
  if (session.passwordSaveQuestion === true) return { kind: "password-save", key: `${noticeKeyPrefix(session.id)}password-save` };
  if (asksNotificationPermission) return { kind: "notification-permission", key: `${noticeKeyPrefix(session.id)}notification-permission` };
  return null;
}

// noticeKeyPrefix starts the key of every notice of a session, so forgetting the notices of one session can find them.
export function noticeKeyPrefix(sessionId: string): string {
  return `${sessionId}\n`;
}

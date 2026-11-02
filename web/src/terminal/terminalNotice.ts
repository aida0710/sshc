import type { TerminalSession } from "../api/terminalSessions";
import { exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";

// TerminalNotice is a state of a session shown in a card over the terminal as well as in the strip above it, for a
// state the person is likely to act on. key names the occurrence, so a card hidden by the person stays hidden until
// the state happens again.
export type TerminalNotice =
  | { kind: "ended"; key: string }
  | { kind: "reconnecting"; key: string }
  | { kind: "link-stopped"; key: string };

// terminalNoticeFor picks the card for a session, if any. A clean exit (the person typed exit) and a link to the engine
// that is still retrying by itself stay in the strip alone.
export function terminalNoticeFor(session: TerminalSession, link: StreamLink): TerminalNotice | null {
  if (session.state === "exited" && session.exited !== undefined) {
    if (session.problem === "" && !exitedAbnormally(session)) return null;
    return { kind: "ended", key: `${noticeKeyPrefix(session.id)}ended:${session.exited.at}` };
  }
  if (session.state === "reconnecting") return { kind: "reconnecting", key: `${noticeKeyPrefix(session.id)}reconnecting` };
  if (link.phase === "stopped" && !link.gone) return { kind: "link-stopped", key: `${noticeKeyPrefix(session.id)}link-stopped` };
  return null;
}

// noticeKeyPrefix starts the key of every notice of a session, so forgetting the notices of one session can find them.
export function noticeKeyPrefix(sessionId: string): string {
  return `${sessionId}\n`;
}

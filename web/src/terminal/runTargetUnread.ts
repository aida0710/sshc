import type { TerminalSession } from "../api/terminalSessions";

// NotificationVersions is what this browser has seen of one run target: each session's notificationVersion.
export type NotificationVersions = ReadonlyMap<string, number>;

// noteNotificationVersions compares a run target's sessions with what was seen before. It reports a new
// notification only for a session seen before whose version moved; the first look at a run target, and a
// session that appeared since, are history rather than news.
export function noteNotificationVersions(
  seen: NotificationVersions | undefined,
  sessions: readonly Pick<TerminalSession, "id" | "notificationVersion">[],
): { seen: NotificationVersions; notified: boolean } {
  const next = new Map<string, number>();
  let notified = false;
  for (const session of sessions) {
    const version = session.notificationVersion ?? 0;
    next.set(session.id, version);
    const previous = seen?.get(session.id);
    if (previous !== undefined && version > previous) notified = true;
  }
  return { seen: next, notified };
}

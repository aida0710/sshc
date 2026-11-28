import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { isUserAway } from "../ui/userAway";
import { announceBlockedProgram, closeSessionNotification } from "./sessionNotificationAnnouncement";
import { blockedProgramNotification } from "./terminalNotifications";
import { useOpenNotifiedSession } from "./useOpenNotifiedSession";

// useBlockedProgramNotifications notifies a person who is away that a program started waiting for them, and takes that
// back once it stops. A browser has only this. The Android app's own engine does it for its sessions (blocked_program.go),
// but a run target's sessions are in another engine, which only this page reads. So on a run target this reach ends where
// the page's does: it hears nothing while Android keeps the page's JavaScript stopped.
export function useBlockedProgramNotifications(
  sessions: TerminalSession[],
  { enabled, t, onOpenSession }: { enabled: boolean; t: Translate; onOpenSession: (id: string) => void },
): void {
  const openNotifiedSession = useOpenNotifiedSession(sessions, t, onOpenSession);
  // blocked is whether each session's program waited when last seen. The first sight of a session is history.
  const blocked = useRef(new Map<string, boolean>());

  useEffect(() => {
    if (!enabled) {
      blocked.current.clear();
      return;
    }
    for (const id of blocked.current.keys()) {
      if (!sessions.some((session) => session.id === id)) blocked.current.delete(id);
    }
    for (const session of sessions) {
      const notification = blockedProgramNotification(t, session);
      const wasBlocked = blocked.current.get(session.id);
      blocked.current.set(session.id, notification !== null);
      if (wasBlocked === undefined || wasBlocked === (notification !== null)) continue;
      if (notification === null) {
        closeSessionNotification(session.id);
      } else if (isUserAway()) {
        announceBlockedProgram({ sessionId: session.id, notification, openNotifiedSession });
      }
    }
  }, [enabled, sessions, t, openNotifiedSession]);
}

import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { closeBrowserNotification, showBrowserNotification } from "../ui/browserNotifications";
import { isUserAway } from "../ui/userAway";
import { terminalNotificationTag } from "./notificationTags";
import { playNotificationSound } from "./notificationSound";
import { blockedProgramNotification } from "./terminalNotifications";

// useBlockedProgramNotifications notifies a person who is away that a program started waiting for them, and takes that
// back once it stops. The Android app's own engine does this for its sessions (blocked_program.go), but a run target's
// sessions are in another engine, which only this page reads. So on a run target this reach ends where the page's does:
// it hears nothing while Android keeps the page's JavaScript stopped.
export function useBlockedProgramNotifications(sessions: TerminalSession[], { enabled, t }: { enabled: boolean; t: Translate }): void {
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
      const tag = terminalNotificationTag(session.id);
      if (notification === null) {
        closeBrowserNotification(tag);
      } else if (isUserAway()) {
        playNotificationSound();
        showBrowserNotification({ ...notification, tag });
      }
    }
  }, [enabled, sessions, t]);
}

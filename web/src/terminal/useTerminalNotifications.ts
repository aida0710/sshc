import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { showBrowserNotification } from "../ui/browserNotifications";
import { playNotificationSound, primeNotificationSound } from "./notificationSound";
import { nextTerminalNotification, type UnreadSessions } from "./terminalNotifications";

// useTerminalNotifications turns notificationVersion changes into unread marks,
// sounds and browser notifications. A pane the user is looking at never
// becomes unread; a hidden or background pane does, until it is focused.
export function useTerminalNotifications(
  sessions: TerminalSession[],
  activeSessionId: string | null,
  t: Translate,
  onOpenSession: (id: string) => void,
): UnreadSessions {
  const seen = useRef(new Map<string, number>());
  const activeSession = useRef(activeSessionId);
  const openSession = useRef(onOpenSession);
  const [unread, setUnread] = useState<Set<string>>(() => new Set());
  activeSession.current = activeSessionId;
  openSession.current = onOpenSession;

  useEffect(() => {
    const prime = () => {
      primeNotificationSound();
      window.removeEventListener("pointerdown", prime, true);
      window.removeEventListener("keydown", prime, true);
    };
    window.addEventListener("pointerdown", prime, true);
    window.addEventListener("keydown", prime, true);
    return () => {
      window.removeEventListener("pointerdown", prime, true);
      window.removeEventListener("keydown", prime, true);
    };
  }, []);

  const markActiveRead = useCallback(() => {
    const id = activeSession.current;
    if (id === null || document.hidden) return;
    setUnread((current) => {
      if (!current.has(id)) return current;
      const next = new Set(current);
      next.delete(id);
      return next;
    });
  }, []);

  useEffect(() => {
    markActiveRead();
    window.addEventListener("focus", markActiveRead);
    document.addEventListener("visibilitychange", markActiveRead);
    return () => {
      window.removeEventListener("focus", markActiveRead);
      document.removeEventListener("visibilitychange", markActiveRead);
    };
  }, [activeSessionId, markActiveRead]);

  useEffect(() => {
    const currentIds = new Set(sessions.map((session) => session.id));
    for (const id of seen.current.keys()) {
      if (!currentIds.has(id)) seen.current.delete(id);
    }
    for (const session of sessions) {
      const previous = seen.current.get(session.id);
      seen.current.set(session.id, session.notificationVersion ?? 0);
      if (previous === undefined) continue;
      const notification = nextTerminalNotification(t, session, previous);
      if (notification === null) continue;
      if (document.hidden || session.id !== activeSession.current) {
        setUnread((current) => {
          if (current.has(session.id)) return current;
          const next = new Set(current);
          next.add(session.id);
          return next;
        });
      }
      // Sounds and browser notifications only reach a user who is away;
      // a visible app already shows the unread mark.
      if (!document.hidden) continue;
      playNotificationSound();
      showBrowserNotification({
        title: notification.title,
        body: notification.body,
        tag: `sshc-terminal-${session.id}`,
        onClick: () => openSession.current(session.id),
      });
    }
    setUnread((current) => {
      if ([...current].every((id) => currentIds.has(id))) return current;
      return new Set([...current].filter((id) => currentIds.has(id)));
    });
  }, [sessions, t]);

  return unread;
}

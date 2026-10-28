import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { closeBrowserNotification, showBrowserNotification } from "../ui/browserNotifications";
import { isUserAway } from "../ui/userAway";
import { useToast } from "../ui/useToast";
import { subscribeAndroidNotificationOpened } from "../android/native";
import { sessionIdFromNotificationTag, terminalNotificationTag } from "./notificationTags";
import { playNotificationSound, primeNotificationSound } from "./notificationSound";
import { nextTerminalNotification, type UnreadSessions } from "./terminalNotifications";

function closeSessionNotifications(sessionId: string): void {
  closeBrowserNotification(terminalNotificationTag(sessionId));
}

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
  const showToast = useToast();
  const currentSessions = useRef(sessions);
  currentSessions.current = sessions;
  // A tapped Android notification can arrive before the session list has
  // loaded (the app was closed or the page reloaded); it opens once it can.
  const [tappedSessionId, setTappedSessionId] = useState<string | null>(null);
  activeSession.current = activeSessionId;
  openSession.current = onOpenSession;

  useEffect(() => subscribeAndroidNotificationOpened((tag) => setTappedSessionId(sessionIdFromNotificationTag(tag))), []);

  useEffect(() => {
    if (tappedSessionId === null || !sessions.some((session) => session.id === tappedSessionId)) return;
    setTappedSessionId(null);
    openSession.current(tappedSessionId);
  }, [sessions, tappedSessionId]);

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
    if (id === null || isUserAway()) return;
    // Seen in the terminal, so the system's notification centre need not keep it.
    closeSessionNotifications(id);
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
      if (currentIds.has(id)) continue;
      seen.current.delete(id);
      closeSessionNotifications(id);
    }
    for (const session of sessions) {
      const previous = seen.current.get(session.id);
      seen.current.set(session.id, session.notificationVersion ?? 0);
      if (previous === undefined) continue;
      const notification = nextTerminalNotification(t, session, previous);
      if (notification === null) continue;
      const away = isUserAway();
      if (away || session.id !== activeSession.current) {
        setUnread((current) => {
          if (current.has(session.id)) return current;
          const next = new Set(current);
          next.add(session.id);
          return next;
        });
      }
      // Sounds and browser notifications only reach a user who is away;
      // a visible app already shows the unread mark.
      if (!away) continue;
      playNotificationSound();
      showBrowserNotification({
        title: notification.title,
        body: notification.body,
        tag: terminalNotificationTag(session.id),
        // The session may have been closed since; then nothing else comes forward.
        onClick: () => {
          if (currentSessions.current.some((current) => current.id === session.id)) openSession.current(session.id);
          else showToast(t("terminal.notificationSessionClosed"));
        },
      });
    }
    setUnread((current) => {
      if ([...current].every((id) => currentIds.has(id))) return current;
      return new Set([...current].filter((id) => currentIds.has(id)));
    });
  }, [sessions, t, showToast]);

  return unread;
}

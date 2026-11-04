import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { showAndroidBlockedProgramNotification } from "../android/native";
import type { Translate } from "../i18n/context";
import { notificationsComeFromAndroid, showBrowserNotification, type BrowserNotification } from "../ui/browserNotifications";
import { terminalNotificationTag } from "./notificationTags";
import { playNotificationSound } from "./notificationSound";
import type { SeenProgramStatus } from "./programStatus";
import { blockedProgramNotification } from "./terminalNotifications";
import { useOpenNotifiedSession } from "./useOpenNotifiedSession";

// A person's own Android settings give a waiting program's channel its sound, so only a browser plays one.
function announce(notification: BrowserNotification): void {
  if (notificationsComeFromAndroid()) {
    showAndroidBlockedProgramNotification(notification);
    return;
  }
  playNotificationSound();
  showBrowserNotification(notification);
}

// useUnseenBlockedProgramNotifications notifies a person who leaves sshc about programs that started waiting while they
// were on another session or page. Only the sidebar told them, and a notification comes only for a change with sshc out
// of sight (useBlockedProgramNotifications, Android's BlockedProgramWatcher), so nothing else will: the program just
// goes on waiting. A status is announced once, however many times the person comes and goes, and Android keeps to one
// for a wait it has announced.
export function useUnseenBlockedProgramNotifications(
  sessions: TerminalSession[],
  seen: SeenProgramStatus,
  { t, onOpenSession }: { t: Translate; onOpenSession: (id: string) => void },
): void {
  const openNotifiedSession = useOpenNotifiedSession(sessions, t, onOpenSession);
  const latest = useRef({ sessions, seen, t, openNotifiedSession });
  latest.current = { sessions, seen, t, openNotifiedSession };
  const announced = useRef(new Map<string, number>());

  useEffect(() => {
    function announceUnseen() {
      const { sessions: current, seen: seenNow, t: translate, openNotifiedSession: open } = latest.current;
      for (const session of current) {
        const notification = blockedProgramNotification(translate, session);
        const version = session.programStatus?.version;
        if (notification === null || version === undefined) continue;
        if ((seenNow.get(session.id) ?? 0) >= version || announced.current.get(session.id) === version) continue;
        announced.current.set(session.id, version);
        announce({ ...notification, tag: terminalNotificationTag(session.id), onClick: () => open(session.id) });
      }
    }
    function announceOnHiding() {
      if (document.hidden) announceUnseen();
    }
    // A PC's window can lose the focus and stay in sight. Android's window loses it for its notification shade, which is
    // not leaving the app.
    function announceOnLosingFocus() {
      if (!document.hasFocus()) announceUnseen();
    }
    const losesFocusByLeaving = !notificationsComeFromAndroid();
    document.addEventListener("visibilitychange", announceOnHiding);
    if (losesFocusByLeaving) window.addEventListener("blur", announceOnLosingFocus);
    return () => {
      document.removeEventListener("visibilitychange", announceOnHiding);
      if (losesFocusByLeaving) window.removeEventListener("blur", announceOnLosingFocus);
    };
  }, []);
}

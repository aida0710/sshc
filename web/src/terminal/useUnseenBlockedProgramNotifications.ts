import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { showAndroidBlockedProgramNotification } from "../android/native";
import type { Translate } from "../i18n/context";
import { terminalNotificationTag } from "./notificationTags";
import type { SeenProgramStatus } from "./programStatus";
import { blockedProgramNotification } from "./terminalNotifications";

// useUnseenBlockedProgramNotifications notifies a person who leaves the app about programs that started waiting while
// they were on another session or page. Only the sidebar told them, and Android's notification comes only for a change
// with the app out of sight (BlockedProgramWatcher), so nothing else will: the program just goes on waiting. A status
// is announced once, however many times the person comes and goes, and the app keeps to one for a wait it has announced.
export function useUnseenBlockedProgramNotifications(
  sessions: TerminalSession[],
  seen: SeenProgramStatus,
  { enabled, t }: { enabled: boolean; t: Translate },
): void {
  const latest = useRef({ sessions, seen, t });
  latest.current = { sessions, seen, t };
  const announced = useRef(new Map<string, number>());

  useEffect(() => {
    if (!enabled) return;
    function announceOnLeaving() {
      if (!document.hidden) return;
      const { sessions: current, seen: seenNow, t: translate } = latest.current;
      for (const session of current) {
        const notification = blockedProgramNotification(translate, session);
        const version = session.programStatus?.version;
        if (notification === null || version === undefined) continue;
        if ((seenNow.get(session.id) ?? 0) >= version || announced.current.get(session.id) === version) continue;
        announced.current.set(session.id, version);
        showAndroidBlockedProgramNotification({ ...notification, tag: terminalNotificationTag(session.id) });
      }
    }
    document.addEventListener("visibilitychange", announceOnLeaving);
    return () => document.removeEventListener("visibilitychange", announceOnLeaving);
  }, [enabled]);
}

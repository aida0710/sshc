import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { isUserAway } from "../ui/userAway";
import type { SeenProgramStatus } from "./programStatus";

// useSeenProgramStatus remembers, per session, the newest program status this
// browser has shown with the session on screen and the person looking: when
// the session is opened, when its status changes while it is open, and when
// the page comes back into view. Like unread notifications, it is this
// browser's own and starts empty on load.
export function useSeenProgramStatus(sessions: TerminalSession[], activeSessionId: string | null): SeenProgramStatus {
  const [seen, setSeen] = useState<ReadonlyMap<string, number>>(() => new Map());
  const currentSessions = useRef(sessions);
  currentSessions.current = sessions;
  const activeSession = useRef(activeSessionId);
  activeSession.current = activeSessionId;

  const markActiveSeen = useCallback(() => {
    const id = activeSession.current;
    if (id === null || isUserAway()) return;
    const version = currentSessions.current.find((session) => session.id === id)?.programStatus?.version;
    if (version === undefined) return;
    setSeen((current) => ((current.get(id) ?? 0) >= version ? current : new Map(current).set(id, version)));
  }, []);

  useEffect(() => {
    markActiveSeen();
    // A closed session's versions are never compared again.
    const ids = new Set(sessions.map((session) => session.id));
    setSeen((current) => {
      if ([...current.keys()].every((id) => ids.has(id))) return current;
      return new Map([...current].filter(([id]) => ids.has(id)));
    });
  }, [sessions, activeSessionId, markActiveSeen]);

  useEffect(() => {
    window.addEventListener("focus", markActiveSeen);
    document.addEventListener("visibilitychange", markActiveSeen);
    return () => {
      window.removeEventListener("focus", markActiveSeen);
      document.removeEventListener("visibilitychange", markActiveSeen);
    };
  }, [markActiveSeen]);

  return seen;
}

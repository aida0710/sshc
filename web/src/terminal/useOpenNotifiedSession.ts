import { useCallback, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { useToast } from "../ui/useToast";

// useOpenNotifiedSession returns what clicking a browser notification does: it opens the session, or says that the
// session is gone. A notification outlives what it announced, so the session may have been closed since it was shown.
export function useOpenNotifiedSession(sessions: TerminalSession[], t: Translate, onOpenSession: (id: string) => void): (id: string) => void {
  const showToast = useToast();
  const latest = useRef({ sessions, t, onOpenSession, showToast });
  latest.current = { sessions, t, onOpenSession, showToast };
  return useCallback((id: string) => {
    const current = latest.current;
    if (current.sessions.some((session) => session.id === id)) current.onOpenSession(id);
    else current.showToast(current.t("terminal.notificationSessionClosed"));
  }, []);
}

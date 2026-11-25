import { useEffect } from "react";
import type { TerminalSessionsState } from "../terminal/sessions";

type Options = {
  // terminalFace is whether the terminal is the section shown.
  terminalFace: boolean;
  // activeSessionId is the session the terminal shows or is about to show. A session opened from Home is shown
  // before the list is read again, so for a moment the list is still empty while this already names it.
  activeSessionId: string | null;
  terminalSessions: Pick<TerminalSessionsState, "sessions" | "loaded" | "opening" | "remoteRefusal">;
  // showHome puts Home in the terminal's place in the history, so Back does not come to the empty terminal again.
  showHome: () => void;
};

// useHomeWithoutSessions shows Home in place of the terminal while the engine in use has no session: after the last
// one is closed, when a phone's Terminal tab is chosen with none open, and when the run target changes to an engine
// with none. The terminal keeps the screen while it has something else to say: the engine has not answered yet, a
// session is being opened or about to be shown, or the run target refuses.
export function useHomeWithoutSessions({ terminalFace, activeSessionId, terminalSessions, showHome }: Options): void {
  const { sessions, loaded, opening, remoteRefusal } = terminalSessions;
  const nothingToShow = terminalFace && activeSessionId === null && loaded && !opening && remoteRefusal === null && sessions.length === 0;
  useEffect(() => {
    if (nothingToShow) showHome();
  }, [nothingToShow, showHome]);
}

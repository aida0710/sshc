import { useCallback, useEffect, useState } from "react";
import type { Section } from "../routing/sectionRoute";
import { runTerminalAction, type TerminalAction } from "../terminal/terminalActions";

// usePendingTerminalAction runs a terminal's action from outside the terminal: it shows the terminal screen, and
// runs the action once that screen is drawn and what asked (the command palette) is gone. Run at once, an action
// that moves the keyboard would find the terminal still hidden, or lose the keyboard to the palette's focus trap.
export function usePendingTerminalAction({ terminalFace, activeSessionId, navigate }: {
  terminalFace: boolean;
  activeSessionId: string | null;
  navigate: (section: Section) => boolean;
}): (action: TerminalAction) => void {
  const [pending, setPending] = useState<TerminalAction | null>(null);
  useEffect(() => {
    if (pending === null || !terminalFace || activeSessionId === null) return;
    setPending(null);
    runTerminalAction(activeSessionId, pending);
  }, [activeSessionId, pending, terminalFace]);
  return useCallback((action) => {
    // A page with unsaved changes can keep the screen as it is; the action is then not wanted any more.
    if (navigate("Terminal")) setPending(action);
  }, [navigate]);
}

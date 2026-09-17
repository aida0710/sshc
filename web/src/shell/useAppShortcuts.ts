import { useEffect } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { matchesShortcut, shortcutKey, shortcutsBlocked, type Bindings } from "../keyconfig/bindings";
import type { Section } from "../routing/sectionRoute";

// Application-wide keyboard shortcuts: the command palette, jumping to a
// section, and cycling consoles. Listened for in the capture phase so that
// xterm never turns one of them into SSH input.
export function useAppShortcuts({ enabled, shortcuts, terminalFace, orderedConsoles, activeConsole, navigate, showConsole, openPalette }: {
  enabled: boolean;
  shortcuts: Bindings;
  terminalFace: boolean;
  orderedConsoles: TerminalSession[];
  activeConsole: string | null;
  navigate: (section: Section) => void;
  showConsole: (id: string) => void;
  openPalette: () => void;
}) {
  useEffect(() => {
    function handleShortcut(event: KeyboardEvent) {
      if (!enabled || shortcutKey(event) === null) return;
      const browserFind = terminalFace && !event.altKey && !event.shiftKey &&
        (event.ctrlKey !== event.metaKey) && event.key.toLowerCase() === "f";
      if (shortcutsBlocked(event)) {
        // Keep confirmation dialogs in place, but do not open browser Find behind them.
        if (browserFind && !(event.target instanceof Element && event.target.closest("[data-shortcut-editor]"))) {
          event.preventDefault();
          event.stopImmediatePropagation();
        }
        return;
      }
      let action: (() => void) | undefined;
      if (matchesShortcut(event, "palette", shortcuts)) {
        action = openPalette;
      } else if (matchesShortcut(event, "home", shortcuts)) action = () => navigate("Home");
      else if (matchesShortcut(event, "sftp", shortcuts)) action = () => navigate("Files");
      else if (orderedConsoles.length > 0) {
        const delta = matchesShortcut(event, "nextSession", shortcuts) ? 1 : matchesShortcut(event, "previousSession", shortcuts) ? -1 : 0;
        if (delta !== 0) action = () => {
          const current = orderedConsoles.findIndex((session) => session.id === activeConsole);
          const index = current < 0 ? (delta > 0 ? 0 : orderedConsoles.length - 1) : (current + delta + orderedConsoles.length) % orderedConsoles.length;
          const selected = orderedConsoles[index];
          if (selected !== undefined) showConsole(selected.id);
        };
      }
      if (action === undefined && !browserFind) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      if (!event.repeat) action?.();
    }
    // Capture before xterm translates an application shortcut into SSH input.
    document.addEventListener("keydown", handleShortcut, true);
    return () => document.removeEventListener("keydown", handleShortcut, true);
  }, [enabled, shortcuts, navigate, orderedConsoles, activeConsole, showConsole, terminalFace, openPalette]);
}

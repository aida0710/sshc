import { useLayoutEffect } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { matchesShortcut, shortcutKey, shortcutsBlocked, type Bindings } from "../keyconfig/bindings";
import type { Section } from "../routing/sectionRoute";

// SessionShortcut is what the new session and close session shortcuts ask of the session list
// (WorkspaceTree): its picker for a new session, or its confirmation before closing one.
export type SessionShortcut = { action: "newSession" } | { action: "closeSession"; sessionId: string };
// SessionShortcutRequest numbers each press, so pressing the same shortcut again asks again.
export type SessionShortcutRequest = SessionShortcut & { serial: number };

// cycled is the item delta places away from current in items, going round at either end. Without a current
// one, forwards starts at the first and backwards at the last.
function cycled(items: readonly string[], current: string | null, delta: number): string | undefined {
  const index = current === null ? -1 : items.indexOf(current);
  if (index < 0) return items[delta > 0 ? 0 : items.length - 1];
  return items[(index + delta + items.length) % items.length];
}

// Application-wide keyboard shortcuts: the command palette, jumping to a
// section, opening and closing sessions, cycling terminal sessions and the panes
// of a split, and showing or hiding the navigation. Listened for in the capture phase
// so that xterm never turns one of them into SSH input.
export function useAppShortcuts({ enabled, shortcuts, terminalFace, orderedSessions, activeSessionId, panes, navigate, showSession, openPalette, requestSession, toggleNavigation }: {
  enabled: boolean;
  shortcuts: Bindings;
  terminalFace: boolean;
  orderedSessions: TerminalSession[];
  activeSessionId: string | null;
  // panes are the sessions of the split shown on the terminal screen, in order; one or none outside a split.
  panes: readonly string[];
  navigate: (section: Section) => void;
  showSession: (id: string) => void;
  openPalette: () => void;
  requestSession: (request: SessionShortcut) => void;
  toggleNavigation: () => void;
}) {
  // The listener closes over `enabled` and the bindings, so it is replaced in a layout
  // effect, in the same task that puts the new screen on the page. A passive effect
  // runs after paint: a key pressed on the freshly shown ready screen would still reach
  // the listener from the previous render (enabled=false) and be dropped.
  useLayoutEffect(() => {
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
      else if (matchesShortcut(event, "toggleNavigation", shortcuts)) action = toggleNavigation;
      else if (matchesShortcut(event, "newSession", shortcuts)) action = () => requestSession({ action: "newSession" });
      else if (terminalFace && activeSessionId !== null && matchesShortcut(event, "closeSession", shortcuts)) {
        action = () => requestSession({ action: "closeSession", sessionId: activeSessionId });
      } else if (terminalFace && panes.length > 1 && (matchesShortcut(event, "nextPane", shortcuts) || matchesShortcut(event, "previousPane", shortcuts))) {
        const pane = cycled(panes, activeSessionId, matchesShortcut(event, "nextPane", shortcuts) ? 1 : -1);
        action = () => {
          if (pane !== undefined) showSession(pane);
        };
      } else if (orderedSessions.length > 0) {
        const delta = matchesShortcut(event, "nextSession", shortcuts) ? 1 : matchesShortcut(event, "previousSession", shortcuts) ? -1 : 0;
        if (delta !== 0) action = () => {
          const selected = cycled(orderedSessions.map((session) => session.id), activeSessionId, delta);
          if (selected !== undefined) showSession(selected);
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
  }, [enabled, shortcuts, navigate, orderedSessions, activeSessionId, panes, showSession, terminalFace, openPalette, requestSession, toggleNavigation]);
}

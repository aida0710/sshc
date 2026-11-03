import { useLayoutEffect } from "react";
import { matchesShortcut, shortcutKey, shortcutsBlocked, type Bindings, type ShortcutAction } from "../keyconfig/bindings";
import { appActions, type AppActionContext, type SessionShortcut } from "./appActions";

// SessionShortcutRequest numbers each press, so pressing the same shortcut again asks again.
export type SessionShortcutRequest = SessionShortcut & { serial: number };

// Application-wide keyboard shortcuts (appActions). Listened for in the capture phase so that xterm never turns
// one of them into SSH input.
export function useAppShortcuts({ enabled, shortcuts, terminalFace, orderedSessions, activeSessionId, panes, splitBesideActive, navigate, showSession, openPalette, requestSession, toggleNavigation }: AppActionContext & {
  enabled: boolean;
  shortcuts: Bindings;
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
      const actions = appActions({ terminalFace, orderedSessions, activeSessionId, panes, splitBesideActive, navigate, showSession, openPalette, requestSession, toggleNavigation });
      const pressed = (Object.keys(actions) as ShortcutAction[]).find((action) => matchesShortcut(event, action, shortcuts));
      if (pressed === undefined && !browserFind) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      if (!event.repeat && pressed !== undefined) actions[pressed]?.();
    }
    // Capture before xterm translates an application shortcut into SSH input.
    document.addEventListener("keydown", handleShortcut, true);
    return () => document.removeEventListener("keydown", handleShortcut, true);
  }, [enabled, shortcuts, navigate, orderedSessions, activeSessionId, panes, splitBesideActive, showSession, terminalFace, openPalette, requestSession, toggleNavigation]);
}

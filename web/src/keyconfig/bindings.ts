import { isValidShortcutChord } from "../rules/shortcutChord";
import { shortcutActions } from "../rules/shortcuts.generated";
import { isImeComposing } from "../ui/imeComposition";

export { shortcutActions };
export type ShortcutAction = (typeof shortcutActions)[number];
export type Bindings = Record<ShortcutAction, string[]>;
// `satisfies` refuses a default for an action the engine does not store and
// a missing default for one it does.
export const defaultBindings = {
  palette: ["Ctrl+K", "Meta+K"],
  terminalSearch: ["Ctrl+F", "Meta+F"],
  copy: ["Ctrl+Shift+C", "Meta+C"],
  paste: ["Ctrl+V", "Meta+V", "Ctrl+Shift+V"],
  // As Ghostty: Ctrl+Shift with the arrows, and Command with them on a Mac.
  previousPrompt: ["Ctrl+Shift+ArrowUp", "Meta+ArrowUp"],
  nextPrompt: ["Ctrl+Shift+ArrowDown", "Meta+ArrowDown"],
  // As Windows Terminal and a browser's page zoom, read only on a terminal. Ctrl+; is the + key of a JIS
  // keyboard, which zooms a page there.
  increaseFontSize: ["Ctrl+=", "Ctrl+;", "Meta+="],
  decreaseFontSize: ["Ctrl+-", "Meta+-"],
  resetFontSize: ["Ctrl+0", "Meta+0"],
  // The keys other terminals use for these (Ctrl+Shift+T, Ctrl+Shift+W, Alt with the arrows) are taken by the
  // browser or by programs in the terminal, so they wait for the user to choose.
  quickCommands: [],
  newSession: [],
  closeSession: [],
  nextSession: ["Alt+PageDown"],
  previousSession: ["Alt+PageUp"],
  nextPane: [],
  previousPane: [],
  splitRight: [],
  splitBelow: [],
  toggleNavigation: [],
  home: [],
  sftp: [],
  // セッションの前後（Alt+PageDown／PageUp）に Shift を足した形。Slack のワークスペースの前後の Ctrl+Tab や
  // ⌘+Shift+[ は、ブラウザがタブの切り替えに使うので取れない。
  nextRunTarget: ["Alt+Shift+PageDown"],
  previousRunTarget: ["Alt+Shift+PageUp"],
  // Slack のワークスペースと同じ番号（Mac は ⌘、ほかは Ctrl）。Ctrl+3〜8 はターミナルでは制御文字になるが、
  // その番号のリモートセッションが無い間は取らない（appActions）。
  runTarget1: ["Ctrl+1", "Meta+1"],
  runTarget2: ["Ctrl+2", "Meta+2"],
  runTarget3: ["Ctrl+3", "Meta+3"],
  runTarget4: ["Ctrl+4", "Meta+4"],
  runTarget5: ["Ctrl+5", "Meta+5"],
  runTarget6: ["Ctrl+6", "Meta+6"],
  runTarget7: ["Ctrl+7", "Meta+7"],
  runTarget8: ["Ctrl+8", "Meta+8"],
  runTarget9: ["Ctrl+9", "Meta+9"],
} satisfies Bindings;

// shortcutKey names the chord a key press makes, or null when it is not one
// the engine stores. Both sides read the grammar from internal/validate, so a
// recorded chord is never refused on save. A '+' key cannot be told apart from
// the separator, so it never makes a chord.
export function shortcutKey(event: KeyboardEvent): string | null {
  if (isImeComposing(event) || event.getModifierState("AltGraph")) return null;
  if (["Control", "Shift", "Alt", "Meta", "Dead", "Unidentified"].includes(event.key)) return null;
  const key = event.key === " " ? "Space" : event.key.length === 1 ? event.key.toUpperCase() : event.key;
  const chord = [event.ctrlKey && "Ctrl", event.altKey && "Alt", event.shiftKey && "Shift", event.metaKey && "Meta", key].filter(Boolean).join("+");
  return isValidShortcutChord(chord) ? chord : null;
}

export function matchesShortcut(event: KeyboardEvent, action: ShortcutAction, bindings: Bindings): boolean {
  const key = shortcutKey(event);
  return key !== null && bindings[action].includes(key);
}
export function shortcutsBlocked(event: KeyboardEvent): boolean {
  return event.defaultPrevented || isImeComposing(event) ||
    (event.target instanceof Element && event.target.closest("[data-shortcut-editor]") !== null) ||
    document.querySelector('[role="dialog"][aria-modal="true"], dialog[open]') !== null;
}

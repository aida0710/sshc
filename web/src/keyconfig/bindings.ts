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
  toggleNavigation: [],
  home: [],
  sftp: [],
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

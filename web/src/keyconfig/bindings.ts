import { useMemo, useSyncExternalStore } from "react";
import { isValidShortcutChord } from "../rules/shortcutChord";
import { maxShortcutKeysPerAction, shortcutActions } from "../rules/shortcuts.generated";
import { readStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";

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
  nextSession: ["Alt+PageDown"],
  previousSession: ["Alt+PageUp"],
  home: [],
  sftp: [],
} satisfies Bindings;
const changedEvent = "sshc-shortcuts-changed";

// shortcutKey names the chord a key press makes, or null when it is not one
// the engine stores. Both sides read the grammar from internal/validate, so a
// recorded chord is never refused on save. A '+' key cannot be told apart from
// the separator, so it never makes a chord.
export function shortcutKey(event: KeyboardEvent): string | null {
  if (event.isComposing || event.keyCode === 229 || event.getModifierState("AltGraph")) return null;
  if (["Control", "Shift", "Alt", "Meta", "Dead", "Unidentified"].includes(event.key)) return null;
  const key = event.key === " " ? "Space" : event.key.length === 1 ? event.key.toUpperCase() : event.key;
  const chord = [event.ctrlKey && "Ctrl", event.altKey && "Alt", event.shiftKey && "Shift", event.metaKey && "Meta", key].filter(Boolean).join("+");
  return isValidShortcutChord(chord) ? chord : null;
}

function validShortcut(value: unknown): value is string {
  return typeof value === "string" && isValidShortcutChord(value);
}

function snapshot(): string {
  return readStoredValue(localStorageKeys.shortcutBindings) ?? "";
}

export function parseBindings(raw: string): Bindings {
  try {
    const value: unknown = JSON.parse(raw);
    if (value === null || typeof value !== "object") return defaultBindings;
    const result: Bindings = { ...defaultBindings };
    for (const action of shortcutActions) {
      const keys = (value as Record<string, unknown>)[action];
      if (Array.isArray(keys) && keys.length <= maxShortcutKeysPerAction && keys.every(validShortcut)) result[action] = [...new Set(keys)];
    }
    // Reject conflicting storage (including hand edits) as a whole.
    const all = Object.values(result).flat();
    return new Set(all).size === all.length ? result : defaultBindings;
  } catch { return defaultBindings; }
}

export function loadBindings(): Bindings { return parseBindings(snapshot()); }
// saveBindings throws when the browser refuses storage instead of swallowing
// it like the preferences in ui/browserStorage: this tab reads its shortcuts
// back from storage, so a refused write leaves the old ones in force. Callers
// decide whether that is worth telling the user.
export function saveBindings(value: Bindings): void {
  window.localStorage.setItem(localStorageKeys.shortcutBindings, JSON.stringify(value));
  window.dispatchEvent(new Event(changedEvent));
}
function subscribe(listener: () => void): () => void {
  window.addEventListener("storage", listener);
  window.addEventListener(changedEvent, listener);
  return () => { window.removeEventListener("storage", listener); window.removeEventListener(changedEvent, listener); };
}
export function useBindings(): Bindings {
  const raw = useSyncExternalStore(subscribe, snapshot, () => "");
  return useMemo(() => parseBindings(raw), [raw]);
}
export function matchesShortcut(event: KeyboardEvent, action: ShortcutAction, bindings?: Bindings): boolean {
  const key = shortcutKey(event);
  return key !== null && (bindings ?? loadBindings())[action].includes(key);
}
export function shortcutsBlocked(event: KeyboardEvent): boolean {
  return event.defaultPrevented || event.isComposing || event.keyCode === 229 ||
    (event.target instanceof Element && event.target.closest("[data-shortcut-editor]") !== null) ||
    document.querySelector('[role="dialog"][aria-modal="true"], dialog[open]') !== null;
}

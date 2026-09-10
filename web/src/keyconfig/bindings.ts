import { useMemo, useSyncExternalStore } from "react";

export const defaultBindings = {
  palette: ["Ctrl+K", "Meta+K"],
  terminalSearch: ["Ctrl+F", "Meta+F"],
  copy: ["Ctrl+Shift+C", "Meta+C"],
  paste: ["Ctrl+V", "Meta+V", "Ctrl+Shift+V"],
  nextSession: ["Alt+PageDown"],
  previousSession: ["Alt+PageUp"],
  home: [],
  sftp: [],
} satisfies Record<string, string[]>;
export type ShortcutAction = keyof typeof defaultBindings;
export type Bindings = Record<ShortcutAction, string[]>;
export const shortcutActions = Object.keys(defaultBindings) as ShortcutAction[];
export const storageKey = "sshc.shortcuts.v1";
const changedEvent = "sshc-shortcuts-changed";
const keyPattern = /^(?:[A-Z0-9]|F(?:[1-9]|1[0-2])|Arrow(?:Up|Down|Left|Right)|PageUp|PageDown|Home|End|Insert|Delete|Backspace|Enter|Tab|Escape|Space|[-=,.;/\[\]\\'`])$/;

export function shortcutKey(event: KeyboardEvent): string | null {
  if (event.isComposing || event.keyCode === 229 || event.getModifierState("AltGraph")) return null;
  if (["Control", "Shift", "Alt", "Meta", "Dead", "Unidentified"].includes(event.key)) return null;
  if (!event.ctrlKey && !event.metaKey && !event.altKey && !/^F([1-9]|1[0-2])$/.test(event.key)) return null;
  const key = event.key === " " ? "Space" : event.key.length === 1 ? event.key.toUpperCase() : event.key;
  // '+' is ambiguous in the serialized format. Use another chord.
  if (!keyPattern.test(key)) return null;
  return [event.ctrlKey && "Ctrl", event.altKey && "Alt", event.shiftKey && "Shift", event.metaKey && "Meta", key].filter(Boolean).join("+");
}

function validShortcut(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const parts = value.split("+");
  const key = parts.pop() ?? "";
  if (!keyPattern.test(key)) return false;
  const event = new KeyboardEvent("keydown", { key: key === "Space" ? " " : key, ctrlKey: parts.includes("Ctrl"), altKey: parts.includes("Alt"), shiftKey: parts.includes("Shift"), metaKey: parts.includes("Meta") });
  return shortcutKey(event) === value;
}

function snapshot(): string {
  try { return window.localStorage.getItem(storageKey) ?? ""; } catch { return ""; }
}

export function parseBindings(raw: string): Bindings {
  try {
    const value: unknown = JSON.parse(raw);
    if (value === null || typeof value !== "object") return defaultBindings;
    const result: Bindings = { ...defaultBindings };
    for (const action of shortcutActions) {
      const keys = (value as Record<string, unknown>)[action];
      if (Array.isArray(keys) && keys.length <= 3 && keys.every(validShortcut)) result[action] = [...new Set(keys)];
    }
    // Upgrade the old default without changing custom bindings or introducing conflicts.
    if (result.paste.length === 2 && result.paste.includes("Ctrl+Shift+V") && result.paste.includes("Meta+V") &&
      !shortcutActions.some((action) => action !== "paste" && result[action].includes("Ctrl+V"))) {
      result.paste = defaultBindings.paste;
    }
    // Reject conflicting storage (including hand edits) as a whole.
    const all = Object.values(result).flat();
    return new Set(all).size === all.length ? result : defaultBindings;
  } catch { return defaultBindings; }
}

export function loadBindings(): Bindings { return parseBindings(snapshot()); }
export function saveBindings(value: Bindings): void {
  window.localStorage.setItem(storageKey, JSON.stringify(value));
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

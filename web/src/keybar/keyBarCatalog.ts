import type { MessageKey } from "../i18n/messages";
import type { IconName } from "../ui/icons";
import { symbolKey } from "./keyBarLayout";
import type { KeyBarKey, KeyBarKeyName } from "./keySequences";

// The keys the key bar can hold, as the bar and the settings page show them. The names come from the engine
// (internal/validate/keybar.go); a name added there has to be given a face here and a group below.

// KeyFace is what a key shows: a short text, an icon where a text's shape or width would not do (arrows, Enter), or
// an icon over a text where the name is too wide for a key of a full row (Shift+Tab).
export type KeyFace =
  | { kind: "text"; text: string }
  | { kind: "icon"; icon: IconName }
  | { kind: "iconOverText"; icon: IconName; text: string };

type KeyLook = {
  face: KeyFace;
  // spoken is the name a screen reader says where the face does not say it well (^C, an arrow).
  spoken?: { message: MessageKey } | { text: string };
};

const text = (value: string): KeyFace => ({ kind: "text", text: value });
const ctrlCombination = (letter: string): KeyLook => ({ face: text(`^${letter}`), spoken: { text: `Ctrl+${letter}` } });

const looks: Record<KeyBarKeyName, KeyLook> = {
  DirectInput: { face: { kind: "icon", icon: "keyboard" }, spoken: { message: "keyBar.directInput" } },
  Control: { face: text("Ctrl") },
  Alt: { face: text("Alt") },
  Escape: { face: text("Esc") },
  Tab: { face: text("Tab") },
  "Shift+Tab": { face: { kind: "iconOverText", icon: "arrowBigUp", text: "Tab" }, spoken: { text: "Shift+Tab" } },
  Enter: { face: { kind: "icon", icon: "cornerDownLeft" }, spoken: { text: "Enter" } },
  Backspace: { face: { kind: "icon", icon: "backspace" }, spoken: { message: "keyBar.backspace" } },
  Delete: { face: text("Del") },
  Insert: { face: text("Ins") },
  ArrowLeft: { face: { kind: "icon", icon: "arrowLeft" }, spoken: { message: "terminal.keyArrowLeft" } },
  ArrowUp: { face: { kind: "icon", icon: "arrowUp" }, spoken: { message: "terminal.keyArrowUp" } },
  ArrowDown: { face: { kind: "icon", icon: "arrowDown" }, spoken: { message: "terminal.keyArrowDown" } },
  ArrowRight: { face: { kind: "icon", icon: "arrowRight" }, spoken: { message: "terminal.keyArrowRight" } },
  Home: { face: text("Home") },
  End: { face: text("End") },
  PageUp: { face: text("PgUp"), spoken: { text: "Page Up" } },
  PageDown: { face: text("PgDn"), spoken: { text: "Page Down" } },
  "Ctrl+C": ctrlCombination("C"),
  "Ctrl+D": ctrlCombination("D"),
  "Ctrl+Z": ctrlCombination("Z"),
  "Ctrl+L": ctrlCombination("L"),
  "Ctrl+R": ctrlCombination("R"),
  "Ctrl+A": ctrlCombination("A"),
  "Ctrl+E": ctrlCombination("E"),
  F1: { face: text("F1") },
  F2: { face: text("F2") },
  F3: { face: text("F3") },
  F4: { face: text("F4") },
  F5: { face: text("F5") },
  F6: { face: text("F6") },
  F7: { face: text("F7") },
  F8: { face: text("F8") },
  F9: { face: text("F9") },
  F10: { face: text("F10") },
  F11: { face: text("F11") },
  F12: { face: text("F12") },
};

// keyLook is how a key shows: a key the person made shows its label.
export function keyLook(key: KeyBarKey): KeyLook {
  return key.key === undefined ? { face: text(key.label ?? "") } : looks[key.key];
}

// PaletteGroup is a heading of the keys the settings page offers to place.
export type PaletteGroup = { heading: MessageKey; keys: KeyBarKey[] };

const named = (...names: KeyBarKeyName[]): KeyBarKey[] => names.map((key) => ({ key }));

// The symbols are offered as keys that type themselves; once placed they are keys like the ones a person makes.
const symbols = ["|", "-", "~", "/", "\\", "_", ":", ";", "`", "*", "&", "$", "@"];

export const paletteGroups: PaletteGroup[] = [
  { heading: "keyBar.groupInput", keys: named("DirectInput", "Control", "Alt", "Escape", "Tab", "Shift+Tab", "Enter") },
  { heading: "keyBar.groupEditing", keys: named("Backspace", "Delete", "Insert", "ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight", "Home", "End", "PageUp", "PageDown") },
  { heading: "keyBar.groupControl", keys: named("Ctrl+C", "Ctrl+D", "Ctrl+Z", "Ctrl+L", "Ctrl+R", "Ctrl+A", "Ctrl+E") },
  { heading: "keyBar.groupFunction", keys: named("F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12") },
  { heading: "keyBar.groupSymbols", keys: symbols.map(symbolKey) },
];

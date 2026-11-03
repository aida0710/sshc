import type { components } from "../api/schema";

// What a key of the key bar sends to the terminal. The sequences are xterm's, as xterm.js sends them for the
// same keys typed on a hardware keyboard, so a program reads a key bar key as that key.

export type KeyBarKey = components["schemas"]["KeyBarKey"];
export type KeyBarKeyName = NonNullable<KeyBarKey["key"]>;

export type KeyModifiers = { ctrl: boolean; alt: boolean };

// TerminalKeyModes is what a program asked of the keys: DECCKM makes the cursor keys, Home and End send SS3
// rather than CSI (vim, less).
export type TerminalKeyModes = { applicationCursorKeys: boolean };

const escape = "\x1b";

// Keys that send one fixed string. Alt puts ESC in front, as xterm's metaSendsEscape does.
const plainKeys: Partial<Record<KeyBarKeyName, string>> = {
  Escape: escape,
  Tab: "\t",
  "Shift+Tab": `${escape}[Z`,
  Enter: "\r",
  "Ctrl+C": "\x03",
  "Ctrl+D": "\x04",
  "Ctrl+Z": "\x1a",
  "Ctrl+L": "\x0c",
  "Ctrl+R": "\x12",
  "Ctrl+A": "\x01",
  "Ctrl+E": "\x05",
};

// Keys that end in a letter: CSI letter, or SS3 letter where the mode asks for it, and CSI 1;m letter with a
// modifier.
const letterKeys: Partial<Record<KeyBarKeyName, { letter: string; cursor: boolean }>> = {
  ArrowUp: { letter: "A", cursor: true },
  ArrowDown: { letter: "B", cursor: true },
  ArrowRight: { letter: "C", cursor: true },
  ArrowLeft: { letter: "D", cursor: true },
  Home: { letter: "H", cursor: true },
  End: { letter: "F", cursor: true },
  // F1 to F4 are SS3 letters whatever the mode.
  F1: { letter: "P", cursor: false },
  F2: { letter: "Q", cursor: false },
  F3: { letter: "R", cursor: false },
  F4: { letter: "S", cursor: false },
};

// Keys that end in a number and a tilde: CSI n ~, and CSI n;m ~ with a modifier.
const tildeKeys: Partial<Record<KeyBarKeyName, number>> = {
  Insert: 2,
  Delete: 3,
  PageUp: 5,
  PageDown: 6,
  F5: 15,
  F6: 17,
  F7: 18,
  F8: 19,
  F9: 20,
  F10: 21,
  F11: 23,
  F12: 24,
};

// modifierParameter is xterm's modifier number: 1, plus 2 for Alt, plus 4 for Ctrl.
function modifierParameter({ ctrl, alt }: KeyModifiers): number {
  return 1 + (alt ? 2 : 0) + (ctrl ? 4 : 0);
}

// SentKeyName is a key that sends something when tapped: not Ctrl or Alt, which are held for the next key, and
// not DirectInput, which switches where typing goes.
type SentKeyName = Exclude<KeyBarKeyName, "Control" | "Alt" | "DirectInput">;

// isModifierKey is whether the key holds Ctrl or Alt for the next key instead of sending anything.
export function isModifierKey(name: KeyBarKeyName): name is "Control" | "Alt" {
  return name === "Control" || name === "Alt";
}

// keySequence is what a key that is not a modifier sends, with the modifiers held for it.
function keySequence(name: SentKeyName, modifiers: KeyModifiers, modes: TerminalKeyModes): string {
  const held = modifiers.ctrl || modifiers.alt;
  const letter = letterKeys[name];
  if (letter !== undefined) {
    if (held) return `${escape}[1;${modifierParameter(modifiers)}${letter.letter}`;
    return letter.cursor && !modes.applicationCursorKeys ? `${escape}[${letter.letter}` : `${escape}O${letter.letter}`;
  }
  const tilde = tildeKeys[name];
  if (tilde !== undefined) return held ? `${escape}[${tilde};${modifierParameter(modifiers)}~` : `${escape}[${tilde}~`;
  // Ctrl+Backspace sends BS, as most terminals do, so a shell can tell it from Backspace.
  const plain = name === "Backspace" ? (modifiers.ctrl ? "\b" : "\x7f") : plainKeys[name] ?? "";
  return modifiers.alt ? escape + plain : plain;
}

// withModifiers is typed text with the key bar's Ctrl and Alt applied: Ctrl turns a letter into its control
// character and Alt puts ESC in front. Text longer than one character (a finished Japanese word) is sent as it is.
export function withModifiers(data: string, { ctrl, alt }: KeyModifiers): string {
  if (data.length !== 1) return data;
  let body = data;
  if (ctrl) {
    const code = data.toLowerCase().charCodeAt(0);
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? escape + body : body;
}

// customKeyInput is what a key the person made sends: its text as if typed, where a line break is a press of
// Enter, then Enter when it asks for one. Ctrl and Alt held for it apply as to a typed key: to a key of one
// character (a symbol), not to a run of text.
function customKeyInput({ text, enter }: { text: string; enter: boolean }, modifiers: KeyModifiers): string {
  const typed = withModifiers(text.replaceAll("\n", "\r"), modifiers);
  return enter ? `${typed}\r` : typed;
}

// keyBarInput is what tapping a key of the key bar sends, with the Ctrl and Alt held for it. Ctrl, Alt and
// DirectInput themselves send nothing.
export function keyBarInput(key: KeyBarKey, modifiers: KeyModifiers, modes: TerminalKeyModes): string {
  if (key.key === undefined) return customKeyInput({ text: key.text ?? "", enter: key.enter ?? false }, modifiers);
  if (isModifierKey(key.key) || key.key === "DirectInput") return "";
  return keySequence(key.key, modifiers, modes);
}

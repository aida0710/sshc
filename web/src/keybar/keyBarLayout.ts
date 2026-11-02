import { maxKeyBarKeys, maxKeyBarMoreKeys } from "../rules/keyBar.generated";
import type { KeyBarKey } from "./keySequences";

// KeyBarLayout is the order of a key bar: the row always shown and the keys behind 「…」. The 「…」 key itself
// is not part of it: it ends the row whenever there are keys behind it.
export type KeyBarLayout = { keys: KeyBarKey[]; moreKeys: KeyBarKey[] };

export type KeyBarRow = keyof KeyBarLayout;

// KeySlot is where a key sits.
export type KeySlot = { row: KeyBarRow; index: number };

const rowLimits: Record<KeyBarRow, number> = { keys: maxKeyBarKeys, moreKeys: maxKeyBarMoreKeys };

// defaultKeyBarLayout is the key bar of a settings preset that chose no key bar preset (the user's choice,
// 2026-11-02).
export const defaultKeyBarLayout: KeyBarLayout = {
  keys: [
    { key: "Escape" }, { key: "Tab" }, { key: "Control" }, { key: "Ctrl+C" },
    { key: "ArrowLeft" }, { key: "ArrowUp" }, { key: "ArrowDown" }, { key: "ArrowRight" },
  ],
  moreKeys: [
    { key: "Alt" }, { key: "Backspace" }, { key: "Home" }, { key: "End" }, { key: "PageUp" }, { key: "PageDown" },
    { label: "|", text: "|" }, { label: "-", text: "-" }, { label: "~", text: "~" }, { label: "/", text: "/" },
  ],
};

// sameKey is whether two keys are the same key: the same named key, or a key the person made with the same
// label and text.
export function sameKey(first: KeyBarKey, second: KeyBarKey): boolean {
  if (first.key !== undefined || second.key !== undefined) return first.key === second.key;
  return first.label === second.label && first.text === second.text && (first.enter ?? false) === (second.enter ?? false);
}

// isPlaced is whether the layout already holds the key. A named key may sit in the bar once.
export function isPlaced(layout: KeyBarLayout, key: KeyBarKey): boolean {
  return [...layout.keys, ...layout.moreKeys].some((placed) => sameKey(placed, key));
}

export function keyAt(layout: KeyBarLayout, slot: KeySlot): KeyBarKey | undefined {
  return layout[slot.row][slot.index];
}

function withRow(layout: KeyBarLayout, row: KeyBarRow, keys: KeyBarKey[]): KeyBarLayout {
  return { ...layout, [row]: keys };
}

// moveKey moves a key one place back (-1) or forward (1) in its row. A key at the end of its row stays.
export function moveKey(layout: KeyBarLayout, slot: KeySlot, step: -1 | 1): { layout: KeyBarLayout; slot: KeySlot } {
  const keys = [...layout[slot.row]];
  const target = slot.index + step;
  const moving = keys[slot.index];
  const other = keys[target];
  if (moving === undefined || other === undefined) return { layout, slot };
  keys[slot.index] = other;
  keys[target] = moving;
  return { layout: withRow(layout, slot.row, keys), slot: { row: slot.row, index: target } };
}

// moveToOtherRow puts a key at the end of the other row, or returns null when that row is full.
export function moveToOtherRow(layout: KeyBarLayout, slot: KeySlot): { layout: KeyBarLayout; slot: KeySlot } | null {
  const moving = keyAt(layout, slot);
  const row: KeyBarRow = slot.row === "keys" ? "moreKeys" : "keys";
  if (moving === undefined || layout[row].length >= rowLimits[row]) return null;
  const from = layout[slot.row].filter((_, index) => index !== slot.index);
  const to = [...layout[row], moving];
  return { layout: withRow(withRow(layout, slot.row, from), row, to), slot: { row, index: to.length - 1 } };
}

export function removeKey(layout: KeyBarLayout, slot: KeySlot): KeyBarLayout {
  return withRow(layout, slot.row, layout[slot.row].filter((_, index) => index !== slot.index));
}

// insertKey puts a key just after the selected one, or at the end of the row always shown when none is
// selected. It returns null when the row is full or the named key is already in the bar.
export function insertKey(layout: KeyBarLayout, after: KeySlot | null, key: KeyBarKey): { layout: KeyBarLayout; slot: KeySlot } | null {
  const row = after?.row ?? "keys";
  if (layout[row].length >= rowLimits[row] || (key.key !== undefined && isPlaced(layout, key))) return null;
  const index = after === null ? layout[row].length : after.index + 1;
  const keys = [...layout[row].slice(0, index), key, ...layout[row].slice(index)];
  return { layout: withRow(layout, row, keys), slot: { row, index } };
}

// replaceKey puts an edited key the person made in place of the old one.
export function replaceKey(layout: KeyBarLayout, slot: KeySlot, key: KeyBarKey): KeyBarLayout {
  return withRow(layout, slot.row, layout[slot.row].map((placed, index) => index === slot.index ? key : placed));
}

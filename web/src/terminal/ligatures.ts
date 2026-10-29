import type { Terminal } from "@xterm/xterm";
import { ligatureSequences } from "./ligatureTable.generated";

// Ligatures draw sequences such as -> and != as one shape. xterm.js draws a
// cell at a time, so a character joiner hands it the runs of symbols that hold
// a ligature, and the font's own rules pick the ligatures in each run. The
// ligatures of JetBrains Mono are as wide as the characters they replace, so
// every character keeps its cell. xterm.js draws a run apart again where the
// cursor or a selection edge falls inside it.

const ligatures = new Set(ligatureSequences);
const longestLigature = Math.max(...ligatureSequences.map((sequence) => sequence.length));
const ligatureCharacters = new Set([...ligatureSequences.join("")]);

// A longer run, such as a separator line of = or -, is drawn a character at a
// time: xterm.js keeps a picture of every run it draws whole, one for each length.
const longestJoinedRun = 16;

function holdsLigature(run: string): boolean {
  for (let start = 0; start < run.length - 1; start += 1) {
    for (let length = 2; length <= longestLigature && start + length <= run.length; length += 1) {
      if (ligatures.has(run.slice(start, start + length))) return true;
    }
  }
  return false;
}

// ligatureRanges are the [start, end) runs of the text to draw as one.
export function ligatureRanges(text: string): [number, number][] {
  const ranges: [number, number][] = [];
  let start = 0;
  while (start < text.length) {
    if (!ligatureCharacters.has(text[start]!)) {
      start += 1;
      continue;
    }
    let end = start + 1;
    while (end < text.length && ligatureCharacters.has(text[end]!)) end += 1;
    if (end - start <= longestJoinedRun && holdsLigature(text.slice(start, end))) ranges.push([start, end]);
    start = end;
  }
  return ranges;
}

export type LigatureDrawing = {
  set(on: boolean): void;
  dispose(): void;
};

// attachLigatures turns ligatures on and off for an opened terminal. The
// attribute gives the DOM renderer's rows the font's ligatures (index.css).
export function attachLigatures(view: Terminal, on: boolean): LigatureDrawing {
  let joiner: number | null = null;
  const set = (value: boolean) => {
    if (value && joiner === null) {
      joiner = view.registerCharacterJoiner(ligatureRanges);
      view.element?.setAttribute("data-ligatures", "");
    } else if (!value && joiner !== null) {
      view.deregisterCharacterJoiner(joiner);
      joiner = null;
      view.element?.removeAttribute("data-ligatures");
    }
  };
  set(on);
  return { set, dispose: () => set(false) };
}

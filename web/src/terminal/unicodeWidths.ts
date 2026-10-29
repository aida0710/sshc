import type { IUnicodeVersionProvider, Terminal } from "@xterm/xterm";
import {
  completesFlag,
  continuesEmojiSequence,
  graphemeBreakStateBits,
  isPlainAfter,
  joinsPreviousCluster,
  nextGraphemeBreakState,
  plainState,
} from "./graphemeBreaks";
import { unicodeVersion, wideRanges, zeroWidthRanges } from "./unicodeWidthTable.generated";

// xterm.js counts character widths with Unicode 6 unless told otherwise, and
// its Unicode 11 addon stops at the emoji of Unicode 12: a newer emoji takes
// one cell while the program that printed it (wcwidth in current C
// libraries, string-width in Node) counted two, and the rest of the line lands
// a column off. These widths come from the current Unicode data instead
// (scripts/unicode/generate-terminal-widths.py).

function inRanges(codepoint: number, ranges: readonly number[]): boolean {
  let low = 0;
  let high = ranges.length / 2 - 1;
  while (low <= high) {
    const middle = (low + high) >> 1;
    if (codepoint < ranges[middle * 2]!) high = middle - 1;
    else if (codepoint > ranges[middle * 2 + 1]!) low = middle + 1;
    else return true;
  }
  return false;
}

let basicPlaneWidths: Uint8Array | null = null;

function basicPlane(): Uint8Array {
  if (basicPlaneWidths !== null) return basicPlaneWidths;
  const widths = new Uint8Array(0x10000).fill(1);
  widths.fill(0, 0, 0x20);
  widths.fill(0, 0x7f, 0xa0);
  const mark = (ranges: readonly number[], width: number) => {
    for (let index = 0; index < ranges.length; index += 2) {
      const first = ranges[index]!;
      if (first >= 0x10000) return;
      widths.fill(width, first, Math.min(ranges[index + 1]!, 0xffff) + 1);
    }
  };
  mark(zeroWidthRanges, 0);
  mark(wideRanges, 2);
  basicPlaneWidths = widths;
  return widths;
}

function cellWidth(codepoint: number): 0 | 1 | 2 {
  if (codepoint < 0x10000) return basicPlane()[codepoint] as 0 | 1 | 2;
  if (inRanges(codepoint, zeroWidthRanges)) return 0;
  return inRanges(codepoint, wideRanges) ? 2 : 1;
}

// The value xterm.js keeps per character, laid out as its own Unicode 6 and
// the Unicode 11 addon lay it out: bit 0 joins the character to the one
// before it, bits 1 and 2 are its width, and the bits above are the
// provider's own state, handed back with the next character.
function characterProperties(width: number, joinsPrevious: boolean, state = 0): number {
  return (state << 3) | (width << 1) | (joinsPrevious ? 1 : 0);
}

function widthOf(properties: number): number {
  return (properties >> 1) & 0x3;
}

const currentUnicode: IUnicodeVersionProvider = {
  version: unicodeVersion,
  wcwidth: cellWidth,
  // A zero-width character (a combining mark) joins the character before it
  // and takes that one's width, as in the Unicode 11 addon.
  charProperties(codepoint, preceding) {
    let width: number = cellWidth(codepoint);
    let joinsPrevious = width === 0 && preceding !== 0;
    if (joinsPrevious) {
      const previousWidth = (preceding >> 1) & 0x3;
      if (previousWidth === 0) joinsPrevious = false;
      else width = Math.max(width, previousWidth);
    }
    return characterProperties(width, joinsPrevious);
  },
};

// With grapheme clusters on (mode 2027, graphemeClusterMode.ts), the characters
// of one user-perceived character share a cell: a family emoji with its zero
// width joiners, a flag of two regional indicators, a hand with a skin tone.
// A cluster is as wide as its widest character, and two cells when it is an
// emoji: after VS16 (U+FE0F), as a flag, or as an emoji ZWJ sequence.
const variationSelector16 = 0xfe0f;

// Set on a zero-width character that started a cluster of its own but was put
// in the cell before, as with clusters off; the next character that would join
// it takes a new cell instead of joining that cell.
const placedBefore = 1 << graphemeBreakStateBits;
const breakStateMask = placedBefore - 1;

const plainProperties = characterProperties(1, false, plainState);

const graphemeClusters: IUnicodeVersionProvider = {
  version: `${unicodeVersion}-graphemes`,
  wcwidth: cellWidth,
  charProperties(codepoint, preceding) {
    const previous = (preceding >> 3) & breakStateMask;
    if (isPlainAfter(previous, codepoint)) return plainProperties;
    const state = nextGraphemeBreakState(previous, codepoint);
    const previousWidth = widthOf(preceding);
    const width = cellWidth(codepoint);
    if (joinsPreviousCluster(state) && previousWidth !== 0 && ((preceding >> 3) & placedBefore) === 0) {
      const emoji = codepoint === variationSelector16 || completesFlag(state) || continuesEmojiSequence(previous, state);
      return characterProperties(emoji ? 2 : Math.max(previousWidth, width), true, state);
    }
    // xterm.js gives every character that does not join a cell of its own,
    // which would move the cursor for a character wcwidth counts as zero.
    if (width === 0 && previousWidth !== 0) return characterProperties(previousWidth, true, state | placedBefore);
    return characterProperties(width, false, state);
  },
};

export function applyCurrentUnicodeWidths(view: Terminal): void {
  view.unicode.register(currentUnicode);
  view.unicode.register(graphemeClusters);
  view.unicode.activeVersion = currentUnicode.version;
}

// setGraphemeClusters switches how the characters printed from now on are
// counted. Those already on the screen keep the cells they were given.
export function setGraphemeClusters(view: Terminal, on: boolean): void {
  view.unicode.activeVersion = on ? graphemeClusters.version : currentUnicode.version;
}

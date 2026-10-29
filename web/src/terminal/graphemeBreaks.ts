import { graphemeBreakRanges } from "./graphemeBreakTable.generated";

// Grapheme cluster boundaries (UAX #29, https://www.unicode.org/reports/tr29/)
// for code points arriving one at a time, as the terminal prints them. The
// state carried from one code point to the next is a number, so that it fits
// in the per-character value xterm.js keeps (see unicodeWidths.ts).

// Grapheme_Cluster_Break values, numbered as scripts/unicode/generate-grapheme-breaks.py writes them.
const Break = {
  Other: 0,
  CR: 1,
  LF: 2,
  Control: 3,
  Extend: 4,
  ZWJ: 5,
  RegionalIndicator: 6,
  Prepend: 7,
  SpacingMark: 8,
  L: 9,
  V: 10,
  T: 11,
  LV: 12,
  LVT: 13,
} as const;

const breakMask = 0xf;
const extendedPictographic = 1 << 4;
const conjunctShift = 5;
// Indic_Conjunct_Break values.
const Conjunct = { None: 0, Consonant: 1, Linker: 2, Extend: 3 } as const;

function classInRanges(codepoint: number): number {
  let low = 0;
  let high = graphemeBreakRanges.length / 3 - 1;
  while (low <= high) {
    const middle = (low + high) >> 1;
    if (codepoint < graphemeBreakRanges[middle * 3]!) high = middle - 1;
    else if (codepoint > graphemeBreakRanges[middle * 3 + 1]!) low = middle + 1;
    else return graphemeBreakRanges[middle * 3 + 2]!;
  }
  return 0;
}

let basicPlaneClasses: Uint8Array | null = null;

function basicPlane(): Uint8Array {
  if (basicPlaneClasses !== null) return basicPlaneClasses;
  const classes = new Uint8Array(0x10000);
  for (let index = 0; index < graphemeBreakRanges.length; index += 3) {
    const first = graphemeBreakRanges[index]!;
    if (first >= 0x10000) break;
    classes.fill(graphemeBreakRanges[index + 2]!, first, Math.min(graphemeBreakRanges[index + 1]!, 0xffff) + 1);
  }
  basicPlaneClasses = classes;
  return classes;
}

function classOf(codepoint: number): number {
  return codepoint < 0x10000 ? basicPlane()[codepoint]! : classInRanges(codepoint);
}

// The state after a code point. 0 is the start of the text.
//   bits 0-3  the Grapheme_Cluster_Break of the code point, plus one
//   bit 4     it ends Extended_Pictographic Extend*            (GB11)
//   bit 5     it is a ZWJ after Extended_Pictographic Extend*  (GB11)
//   bit 6     it ends an odd run of regional indicators        (GB12, GB13)
//   bits 7-8  where it leaves an Indic conjunct                (GB9c):
//             1 after a consonant, 2 after a linker following one
//   bit 9     it joined the cluster of the code point before it
// The value fits in 10 bits.
const previousMask = 0xf;
const emojiRun = 1 << 4;
const zwjAfterEmoji = 1 << 5;
const oddRegionalIndicators = 1 << 6;
const conjunctStateShift = 7;
const conjunctStateMask = 0x3 << conjunctStateShift;
const afterConsonant = 1;
const afterLinker = 2;
const joinedBit = 1 << 9;

export const graphemeBreakStateBits = 10;

export function joinsPreviousCluster(state: number): boolean {
  return (state & joinedBit) !== 0;
}

// completesFlag is whether the code point joined as the second of two regional indicators.
export function completesFlag(state: number): boolean {
  return joinsPreviousCluster(state) && (state & previousMask) === Break.RegionalIndicator + 1;
}

// continuesEmojiSequence is whether the code point joined by GB11: an emoji
// after a zero width joiner that follows another emoji.
export function continuesEmojiSequence(previous: number, state: number): boolean {
  return joinsPreviousCluster(state) && (previous & zwjAfterEmoji) !== 0 && (state & emojiRun) !== 0;
}

// isPlainAfter is whether the code point is ASCII text the state cannot join:
// only GB9b joins anything to an ASCII character, after a Prepend.
export function isPlainAfter(previous: number, codepoint: number): boolean {
  return codepoint >= 0x20 && codepoint < 0x7f && (previous & previousMask) !== Break.Prepend + 1;
}

// The state after printable ASCII, whatever came before, unless isPlainAfter is false.
export const plainState = Break.Other + 1;

function joins(previous: number, kind: number, conjunct: number, pictographic: boolean): boolean {
  if (previous === 0) return false;
  const before = (previous & previousMask) - 1;
  if (before === Break.CR && kind === Break.LF) return true; // GB3
  if (before === Break.CR || before === Break.LF || before === Break.Control) return false; // GB4
  if (kind === Break.CR || kind === Break.LF || kind === Break.Control) return false; // GB5
  if (before === Break.L && (kind === Break.L || kind === Break.V || kind === Break.LV || kind === Break.LVT)) return true; // GB6
  if ((before === Break.LV || before === Break.V) && (kind === Break.V || kind === Break.T)) return true; // GB7
  if ((before === Break.LVT || before === Break.T) && kind === Break.T) return true; // GB8
  if (kind === Break.Extend || kind === Break.ZWJ) return true; // GB9
  if (kind === Break.SpacingMark) return true; // GB9a
  if (before === Break.Prepend) return true; // GB9b
  if (conjunct === Conjunct.Consonant && (previous & conjunctStateMask) >> conjunctStateShift === afterLinker) return true; // GB9c
  if ((previous & zwjAfterEmoji) !== 0 && pictographic) return true; // GB11
  if (before === Break.RegionalIndicator && kind === Break.RegionalIndicator) return (previous & oddRegionalIndicators) !== 0; // GB12, GB13
  return false; // GB999
}

function conjunctState(previous: number, conjunct: number): number {
  const current = (previous & conjunctStateMask) >> conjunctStateShift;
  if (conjunct === Conjunct.Consonant) return afterConsonant;
  if (conjunct === Conjunct.Linker && current !== 0) return afterLinker;
  if (conjunct === Conjunct.Extend) return current;
  return 0;
}

// nextGraphemeBreakState is the state after the code point, given the state
// before it; joinsPreviousCluster tells whether it continues that cluster.
export function nextGraphemeBreakState(previous: number, codepoint: number): number {
  const properties = classOf(codepoint);
  const kind = properties & breakMask;
  const conjunct = properties >> conjunctShift;
  const pictographic = (properties & extendedPictographic) !== 0;
  let state = kind + 1;
  if (pictographic || (kind === Break.Extend && (previous & emojiRun) !== 0)) state |= emojiRun;
  if (kind === Break.ZWJ && (previous & emojiRun) !== 0) state |= zwjAfterEmoji;
  if (kind === Break.RegionalIndicator) {
    const closesPair = (previous & previousMask) === Break.RegionalIndicator + 1 && (previous & oddRegionalIndicators) !== 0;
    if (!closesPair) state |= oddRegionalIndicators;
  }
  state |= conjunctState(previous, conjunct) << conjunctStateShift;
  if (joins(previous, kind, conjunct, pictographic)) state |= joinedBit;
  return state;
}

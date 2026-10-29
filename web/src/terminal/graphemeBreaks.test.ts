import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { joinsPreviousCluster, nextGraphemeBreakState } from "./graphemeBreaks";

// Unicode's own boundary tests (GraphemeBreakTest.txt of the same version as the table),
// written by scripts/unicode/generate-grapheme-breaks.py.
const cases = readFileSync(join(__dirname, "testdata", "grapheme-break-test.txt"), "utf8")
  .split("\n")
  .filter((line) => line !== "" && !line.startsWith("#"));

function boundaries(codepoints: number[]): string[] {
  const marks: string[] = [];
  let state = 0;
  for (const codepoint of codepoints) {
    state = nextGraphemeBreakState(state, codepoint);
    marks.push(joinsPreviousCluster(state) ? "×" : "÷");
  }
  return marks;
}

describe("grapheme cluster boundaries", () => {
  it("are Unicode's in all of its boundary tests", () => {
    expect(cases.length).toBeGreaterThan(700);
    const wrong: string[] = [];
    for (const line of cases) {
      const tokens = line.split(/\s+/);
      const codepoints = tokens.filter((_, index) => index % 2 === 1).map((hex) => Number.parseInt(hex, 16));
      // The first mark is the start of the text, always a boundary.
      const expected = tokens.filter((_, index) => index % 2 === 0).slice(1, -1);
      const found = boundaries(codepoints).slice(1);
      if (found.join(" ") !== expected.join(" ")) wrong.push(line);
    }
    expect(wrong).toEqual([]);
  });
});

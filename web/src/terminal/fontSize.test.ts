import { describe, expect, it } from "vitest";
import { fontSizeRange, pinchedFontSize, steppedFontSize } from "./fontSize";

describe("terminal font size", () => {
  it("steps by whole pixels and stops at the sizes the engine accepts", () => {
    expect(steppedFontSize(13, 1)).toBe(14);
    expect(steppedFontSize(13, -2)).toBe(11);
    expect(steppedFontSize(fontSizeRange.max, 1)).toBe(fontSizeRange.max);
    expect(steppedFontSize(fontSizeRange.min, -1)).toBe(fontSizeRange.min);
  });

  it("grows with the gap between the fingers of a pinch, to a whole pixel", () => {
    expect(pinchedFontSize(14, 1.5)).toBe(21);
    expect(pinchedFontSize(14, 0.5)).toBe(fontSizeRange.min);
    expect(pinchedFontSize(14, 3)).toBe(fontSizeRange.max);
  });
});

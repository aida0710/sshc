import { describe, expect, it } from "vitest";
import { contrastRatio, parseAccentColor, readableInkOn } from "./accentColor";

describe("accent colours", () => {
  it("reads a typed colour with or without the # and in either case", () => {
    expect(parseAccentColor("#3fa86b")).toBe("#3fa86b");
    expect(parseAccentColor(" 3FA86B ")).toBe("#3fa86b");
    expect(parseAccentColor("#3fa86")).toBeNull();
    expect(parseAccentColor("green")).toBeNull();
    expect(parseAccentColor("#3fa86b80")).toBeNull();
  });

  it("puts dark ink on a light colour and white ink on a dark one", () => {
    expect(readableInkOn("#b5a8ff")).toBe("#111214");
    expect(readableInkOn("#ffffff")).toBe("#111214");
    expect(readableInkOn("#6550d0")).toBe("#ffffff");
    expect(readableInkOn("#111214")).toBe("#ffffff");
  });

  it("measures contrast as WCAG does", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrastRatio("#ffffff", "#000000")).toBeCloseTo(21, 5);
    expect(contrastRatio("#777777", "#777777")).toBe(1);
  });
});

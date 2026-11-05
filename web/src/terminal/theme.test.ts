import { describe, expect, it } from "vitest";
import { terminalTheme } from "./theme";

describe("terminalTheme", () => {
  it("draws no border beside the scrollbar", () => {
    expect(terminalTheme().overviewRulerBorder).toBe("#00000000");
  });
});

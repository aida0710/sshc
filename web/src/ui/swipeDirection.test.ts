import { describe, expect, it } from "vitest";
import { swipeDirection } from "./swipeDirection";

describe("swipeDirection", () => {
  it("leaves a finger that has barely moved undecided, so a tap stays a tap", () => {
    expect(swipeDirection(6, -8)).toBe("undecided");
  });

  it("reads a finger moving mostly across as sideways, also when it drifts down", () => {
    expect(swipeDirection(40, 15)).toBe("sideways");
    expect(swipeDirection(-40, -15)).toBe("sideways");
  });

  it("reads a slightly slanted finger moving up or down as up and down", () => {
    expect(swipeDirection(11, -10)).toBe("upDown");
    expect(swipeDirection(-5, 30)).toBe("upDown");
  });
});

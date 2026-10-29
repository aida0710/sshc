import { describe, expect, it } from "vitest";
import { swipeOf } from "./previewSwipe";

describe("swipeOf", () => {
  const start = { x: 200, y: 300 };

  it("goes to the previous file on a swipe to the right and to the next on one to the left", () => {
    expect(swipeOf(start, { x: 320, y: 310 })).toBe("previous");
    expect(swipeOf(start, { x: 80, y: 290 })).toBe("next");
  });

  it("closes on a swipe down, and does nothing on one up", () => {
    expect(swipeOf(start, { x: 210, y: 420 })).toBe("close");
    expect(swipeOf(start, { x: 210, y: 180 })).toBeNull();
  });

  it("takes neither a short stroke nor a diagonal one for a swipe", () => {
    expect(swipeOf(start, { x: 230, y: 305 })).toBeNull();
    expect(swipeOf(start, { x: 300, y: 400 })).toBeNull();
  });
});

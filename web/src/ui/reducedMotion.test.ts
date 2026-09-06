import { describe, expect, it } from "vitest";
import { cursorAnimationEnabled, editorCursorBlinking, reducedMotionQuery } from "./reducedMotion";

describe("reduced motion cursor policy", () => {
  it("uses the platform media query as its single source", () => {
    expect(reducedMotionQuery).toBe("(prefers-reduced-motion: reduce)");
  });

  it("keeps both terminal implementations static under reduced motion", () => {
    expect(cursorAnimationEnabled(true)).toBe(false);
    expect(editorCursorBlinking(true)).toBe("solid");
  });

  it("retains normal cursor animation otherwise", () => {
    expect(cursorAnimationEnabled(false)).toBe(true);
    expect(editorCursorBlinking(false)).toBe("blink");
  });
});

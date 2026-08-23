import { describe, expect, it, vi } from "vitest";
import { prefersNativeSelection } from "./nativeSelection";

describe("prefersNativeSelection", () => {
  it("asks for the screens that have no hover and a coarse pointer", () => {
    const match = vi.fn().mockReturnValue({ matches: true });
    expect(prefersNativeSelection(match)).toBe(true);
    expect(match).toHaveBeenCalledWith("(hover: none) and (pointer: coarse)");
  });

  it("stays out of the way where a mouse exists", () => {
    expect(prefersNativeSelection(() => ({ matches: false }))).toBe(false);
  });
});

import { describe, expect, it } from "vitest";
import { modifierDoubleTapMilliseconds, releaseAfterKey, tapModifier } from "./modifierLatch";

describe("tapModifier", () => {
  it("holds a released modifier for the next key on one tap", () => {
    expect(tapModifier("off", Number.POSITIVE_INFINITY)).toBe("once");
  });

  it("locks the modifier when the second tap comes quickly", () => {
    expect(tapModifier("once", modifierDoubleTapMilliseconds)).toBe("locked");
  });

  it("lets go when the second tap comes slowly", () => {
    expect(tapModifier("once", modifierDoubleTapMilliseconds + 1)).toBe("off");
  });

  it("unlocks a locked modifier on the next tap, however quick", () => {
    expect(tapModifier("locked", 10)).toBe("off");
  });
});

describe("releaseAfterKey", () => {
  it("lets go of a modifier held for one key and keeps a locked one", () => {
    expect(releaseAfterKey({ ctrl: "locked", alt: "once" })).toEqual({ ctrl: "locked", alt: "off" });
  });
});

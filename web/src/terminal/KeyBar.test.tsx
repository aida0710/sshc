import { describe, expect, it } from "vitest";
import { applyModifiers, encodeKey } from "./KeyBar";

describe("encodeKey", () => {
  it("sends the control sequences the keys stand for", () => {
    expect(encodeKey("Esc", false, false)).toBe("\x1b");
    expect(encodeKey("Tab", false, false)).toBe("\t");
    expect(encodeKey("↑", false, false)).toBe("\x1b[A");
    expect(encodeKey("↓", false, false)).toBe("\x1b[B");
    expect(encodeKey("→", false, false)).toBe("\x1b[C");
    expect(encodeKey("←", false, false)).toBe("\x1b[D");
  });

  it("passes literal characters through", () => {
    expect(encodeKey("|", false, false)).toBe("|");
    expect(encodeKey("~", false, false)).toBe("~");
  });

  it("folds ctrl into the control range", () => {
    expect(encodeKey("c", true, false)).toBe("\x03");
    expect(encodeKey("C", true, false)).toBe("\x03");
    expect(encodeKey("d", true, false)).toBe("\x04");
  });

  it("prefixes alt with escape", () => {
    expect(encodeKey("b", false, true)).toBe("\x1bb");
  });

  it("puts the escape before the control character", () => {
    expect(encodeKey("c", true, true)).toBe("\x1b\x03");
  });

  it("leaves the special sequences alone when ctrl is held", () => {
    expect(encodeKey("↑", true, false)).toBe("\x1b[A");
  });

  it("sends the plain character when ctrl has no meaning for it", () => {
    expect(encodeKey("|", true, false)).toBe("|");
  });
});

describe("encodeKey and things that are not one keystroke", () => {
  it("leaves a pasted run alone even when ctrl is armed", () => {
    expect(encodeKey("cd /etc", true, false)).toBe("cd /etc");
  });

  it("leaves an assembled control sequence alone", () => {
    expect(encodeKey("\x1b[A", true, false)).toBe("\x1b[A");
  });
});

describe("the two doors are not the same door", () => {
  it("turns a pressed key into its sequence even with no modifier", () => {
    expect(encodeKey("Esc", false, false)).toBe("\x1b");
    expect(encodeKey("Tab", false, false)).toBe("\t");
    expect(encodeKey("↑", false, false)).toBe("\x1b[A");
  });

  it("leaves typed text alone even when it spells a key name", () => {
    expect(applyModifiers("Esc", false, false)).toBe("Esc");
    expect(applyModifiers("Tab", true, false)).toBe("Tab");
  });

  it("still folds ctrl into a single typed character", () => {
    expect(applyModifiers("c", true, false)).toBe("\x03");
    expect(applyModifiers("a", false, false)).toBe("a");
  });
});

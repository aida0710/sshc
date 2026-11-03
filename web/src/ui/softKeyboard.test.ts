import { afterEach, describe, expect, it, vi } from "vitest";
import { asciiTextEntry, focusRaisesKeyboard, hasOnScreenKeyboard } from "./softKeyboard";
import { mobileViewportQuery } from "./useMediaQuery";

function screenLaidOutAsPhone(phone: boolean) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: phone && query === mobileViewportQuery, media: query })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("focusRaisesKeyboard", () => {
  it("is true for a text field on a phone, and only there", () => {
    const field = document.createElement("textarea");
    screenLaidOutAsPhone(true);
    expect(hasOnScreenKeyboard()).toBe(true);
    expect(focusRaisesKeyboard(field)).toBe(true);
    screenLaidOutAsPhone(false);
    expect(hasOnScreenKeyboard()).toBe(false);
    expect(focusRaisesKeyboard(field)).toBe(false);
  });

  it("is false on a phone for what takes no text", () => {
    screenLaidOutAsPhone(true);
    expect(focusRaisesKeyboard(document.createElement("button"))).toBeFalsy();
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    expect(focusRaisesKeyboard(checkbox)).toBeFalsy();
    expect(focusRaisesKeyboard(null)).toBe(false);
  });
});

describe("asciiTextEntry", () => {
  // Gboard in Japanese stays on kana for inputmode="url", the kind this used to ask for.
  it("asks for the keyboard that opens on Latin letters without correcting them", () => {
    expect(asciiTextEntry).toEqual({ inputMode: "email", autoCapitalize: "off", autoCorrect: "off", spellCheck: false });
  });
});

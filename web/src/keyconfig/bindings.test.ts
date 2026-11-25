import { describe, expect, it } from "vitest";
import { defaultBindings, matchesShortcut, type ShortcutAction } from "./bindings";

describe("matchesShortcut", () => {
  it("takes neither Ctrl+Space nor Meta+Space, the keys that switch the input source, for a change of the text size", () => {
    const fontSizeActions: ShortcutAction[] = ["increaseFontSize", "decreaseFontSize", "resetFontSize"];
    for (const modifier of [{ ctrlKey: true }, { metaKey: true }]) {
      const pressed = new KeyboardEvent("keydown", { key: " ", code: "Space", ...modifier });
      for (const action of fontSizeActions) expect(matchesShortcut(pressed, action, defaultBindings)).toBe(false);
    }
  });
});

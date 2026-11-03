import { afterEach, describe, expect, it, vi } from "vitest";
import { shortcutLabel } from "./shortcutLabel";

describe("shortcutLabel", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("writes the arrow keys as arrows", () => {
    expect(shortcutLabel("Ctrl+Shift+ArrowUp")).toBe("Ctrl+Shift+↑");
    expect(shortcutLabel("Alt+ArrowRight")).toBe("Alt+→");
  });

  it("calls the Meta key by the name on the keyboard in front of the person", () => {
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" });
    expect(shortcutLabel("Meta+K")).toBe("⌘+K");
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" });
    expect(shortcutLabel("Meta+K")).toBe("Win+K");
  });

  it("leaves the other keys as they are", () => {
    expect(shortcutLabel("Alt+PageDown")).toBe("Alt+PageDown");
    expect(shortcutLabel("Ctrl+=")).toBe("Ctrl+=");
  });
});

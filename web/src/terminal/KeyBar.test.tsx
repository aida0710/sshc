import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { applyModifiers, encodeKey, KeyBar } from "./KeyBar";
import { mobileViewportQuery } from "../ui/useMediaQuery";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function showMobileKeyBar() {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches: query === mobileViewportQuery,
    media: query,
    addEventListener: vi.fn(), removeEventListener: vi.fn(),
    addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(), onchange: null,
  })));
  const onKey = vi.fn();
  const onToggle = vi.fn();
  render(<KeyBar modifiers={{ ctrl: false, alt: false }} onKey={onKey} onToggle={onToggle} />);
  return { onKey, onToggle };
}

describe("mobile key bar", () => {
  it("shows common keys in the phone landscape layout and tucks symbols behind extra keys", () => {
    showMobileKeyBar();
    expect(window.matchMedia).toHaveBeenCalledWith(mobileViewportQuery);
    for (const label of ["Ctrl", "Esc", "Tab", "←", "↑", "↓", "→"]) {
      expect(screen.getByRole("button", { name: label })).toBeVisible();
    }
    expect(screen.queryByRole("button", { name: "|" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Extra keys" })).toHaveAttribute("aria-expanded", "false");
  });

  it("sends an extra key and returns to the compact bar without taking input focus", () => {
    const { onKey } = showMobileKeyBar();
    const input = document.createElement("textarea");
    document.body.appendChild(input);
    input.focus();
    const more = screen.getByRole("button", { name: "Extra keys" });
    expect(fireEvent.pointerDown(more, { cancelable: true })).toBe(false);
    fireEvent.click(more);
    const symbol = screen.getByRole("button", { name: "|" });
    expect(fireEvent.pointerDown(symbol, { cancelable: true })).toBe(false);
    fireEvent.click(symbol);
    expect(onKey).toHaveBeenCalledWith("|");
    expect(input).toHaveFocus();
    expect(more).toHaveAttribute("aria-expanded", "false");
    input.remove();
  });

  it("toggles Alt from extra keys and dismisses the popup", () => {
    const { onToggle } = showMobileKeyBar();
    fireEvent.click(screen.getByRole("button", { name: "Extra keys" }));
    fireEvent.click(screen.getByRole("button", { name: "Alt" }));
    expect(onToggle).toHaveBeenCalledWith("alt");
    expect(screen.getByRole("button", { name: "Extra keys" })).toHaveAttribute("aria-expanded", "false");
  });
});

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

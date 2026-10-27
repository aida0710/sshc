import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { applyModifiers, encodeKey, KeyBar } from "./KeyBar";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { modifierDoubleTapMilliseconds, releasedModifiers, type Modifiers } from "./modifierLatch";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); delete document.documentElement.dataset.keyboardOpen; });

function showMobileKeyBar(modifiers: Modifiers = releasedModifiers) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches: query === mobileViewportQuery,
    media: query,
    addEventListener: vi.fn(), removeEventListener: vi.fn(),
    addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(), onchange: null,
  })));
  const onKey = vi.fn();
  const onTap = vi.fn();
  render(<KeyBar modifiers={modifiers} onKey={onKey} onTap={onTap} />);
  return { onKey, onTap };
}

describe("mobile key bar", () => {
  it("shows common keys in the phone landscape layout and tucks symbols behind extra keys", () => {
    showMobileKeyBar();
    expect(window.matchMedia).toHaveBeenCalledWith(mobileViewportQuery);
    for (const label of ["Ctrl", "Esc", "Tab", "Left arrow", "Up arrow", "Down arrow", "Right arrow"]) {
      expect(screen.getByRole("button", { name: label })).toBeVisible();
    }
    expect(screen.queryByRole("button", { name: "|" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Extra keys" })).toHaveAttribute("aria-expanded", "false");
  });

  it("sends an extra key and returns to the compact bar, leaving an open keyboard open", () => {
    const { onKey } = showMobileKeyBar();
    const input = document.createElement("textarea");
    document.body.appendChild(input);
    input.focus();
    document.documentElement.dataset.keyboardOpen = "true";
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

  it("does not bring a closed keyboard up when Ctrl or an arrow is tapped", () => {
    const { onKey, onTap } = showMobileKeyBar();
    const input = document.createElement("textarea");
    document.body.appendChild(input);
    input.focus();
    document.documentElement.dataset.keyboardOpen = "false";
    // Android shows the keyboard on a tap while a text field has the focus, so the field lets go of it.
    const ctrl = screen.getByRole("button", { name: "Ctrl" });
    expect(fireEvent.pointerDown(ctrl, { cancelable: true })).toBe(false);
    fireEvent.click(ctrl);
    expect(onTap).toHaveBeenCalledWith("ctrl");
    expect(input).not.toHaveFocus();
    input.focus();
    const up = screen.getByRole("button", { name: "Up arrow" });
    fireEvent.pointerDown(up, { cancelable: true });
    fireEvent.click(up);
    expect(onKey).toHaveBeenCalledWith("ArrowUp");
    expect(input).not.toHaveFocus();
    expect(up).not.toHaveFocus();
    input.remove();
  });

  it("keeps the popup open for a second tap on Alt, then dismisses it", () => {
    vi.useFakeTimers();
    try {
      const { onTap } = showMobileKeyBar();
      fireEvent.click(screen.getByRole("button", { name: "Extra keys" }));
      fireEvent.click(screen.getByRole("button", { name: "Alt" }));
      fireEvent.click(screen.getByRole("button", { name: "Alt" }));
      expect(onTap).toHaveBeenCalledTimes(2);
      expect(onTap).toHaveBeenCalledWith("alt");
      act(() => { vi.advanceTimersByTime(modifierDoubleTapMilliseconds); });
      expect(screen.getByRole("button", { name: "Extra keys" })).toHaveAttribute("aria-expanded", "false");
    } finally {
      vi.useRealTimers();
    }
  });

  it("tells a modifier held for one key from a locked one", () => {
    showMobileKeyBar({ ctrl: "locked", alt: "once" });
    expect(screen.getByRole("button", { name: "Ctrl (locked)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Extra keys" })).toHaveTextContent("Alt");
  });
});

describe("encodeKey", () => {
  it("sends the control sequences the keys stand for", () => {
    expect(encodeKey("Esc", false, false)).toBe("\x1b");
    expect(encodeKey("Tab", false, false)).toBe("\t");
    expect(encodeKey("ArrowUp", false, false)).toBe("\x1b[A");
    expect(encodeKey("ArrowDown", false, false)).toBe("\x1b[B");
    expect(encodeKey("ArrowRight", false, false)).toBe("\x1b[C");
    expect(encodeKey("ArrowLeft", false, false)).toBe("\x1b[D");
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
    expect(encodeKey("ArrowUp", true, false)).toBe("\x1b[A");
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
    expect(encodeKey("ArrowUp", false, false)).toBe("\x1b[A");
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

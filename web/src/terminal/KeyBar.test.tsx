import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { KeyBar } from "./KeyBar";
import { defaultKeyBarLayout, type KeyBarLayout } from "../keybar/keyBarLayout";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { modifierDoubleTapMilliseconds, releasedModifiers, type Modifiers } from "./modifierLatch";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); delete document.documentElement.dataset.keyboardOpen; });

function showMobileKeyBar(modifiers: Modifiers = releasedModifiers, layout: KeyBarLayout = defaultKeyBarLayout, directInput = false) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches: query === mobileViewportQuery,
    media: query,
    addEventListener: vi.fn(), removeEventListener: vi.fn(),
    addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(), onchange: null,
  })));
  const onKey = vi.fn();
  const onTap = vi.fn();
  const onDirectInput = vi.fn();
  render(<KeyBar layout={layout} modifiers={modifiers} directInput={directInput} onKey={onKey} onTap={onTap} onDirectInput={onDirectInput} />);
  return { onKey, onTap, onDirectInput };
}

describe("mobile key bar", () => {
  it("shows common keys in the phone landscape layout and tucks symbols behind extra keys", () => {
    showMobileKeyBar();
    expect(window.matchMedia).toHaveBeenCalledWith(mobileViewportQuery);
    for (const label of ["Ctrl", "Ctrl+C", "Esc", "Tab", "Left arrow", "Up arrow", "Down arrow", "Right arrow"]) {
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
    expect(onKey).toHaveBeenCalledWith({ label: "|", text: "|" });
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
    expect(onKey).toHaveBeenCalledWith({ key: "ArrowUp" });
    expect(input).not.toHaveFocus();
    expect(up).not.toHaveFocus();
    input.remove();
  });

  it("sends Ctrl+C in one tap, without Ctrl and without bringing a closed keyboard up", () => {
    const { onKey, onTap } = showMobileKeyBar();
    const input = document.createElement("textarea");
    document.body.appendChild(input);
    input.focus();
    document.documentElement.dataset.keyboardOpen = "false";
    const interrupt = screen.getByRole("button", { name: "Ctrl+C" });
    expect(interrupt).toHaveTextContent("^C");
    expect(fireEvent.pointerDown(interrupt, { cancelable: true })).toBe(false);
    fireEvent.click(interrupt);
    expect(onKey).toHaveBeenCalledWith({ key: "Ctrl+C" });
    expect(onTap).not.toHaveBeenCalled();
    expect(input).not.toHaveFocus();
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

  it("lays the keys out as the key bar preset says, with Ctrl and Alt anywhere", () => {
    const { onKey, onTap } = showMobileKeyBar(releasedModifiers, {
      keys: [{ key: "Alt" }, { key: "F5" }, { label: "git st", text: "git status", enter: true }],
      moreKeys: [{ key: "Control" }],
    });
    fireEvent.click(screen.getByRole("button", { name: "Alt" }));
    expect(onTap).toHaveBeenCalledWith("alt");
    fireEvent.click(screen.getByRole("button", { name: "F5" }));
    expect(onKey).toHaveBeenCalledWith({ key: "F5" });
    fireEvent.click(screen.getByRole("button", { name: "git st" }));
    expect(onKey).toHaveBeenCalledWith({ label: "git st", text: "git status", enter: true });
    expect(screen.queryByRole("button", { name: "Esc" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Extra keys" }));
    fireEvent.click(screen.getByRole("button", { name: "Ctrl" }));
    expect(onTap).toHaveBeenCalledWith("ctrl");
  });

  it("leaves out 「…」 when no key sits behind it", () => {
    showMobileKeyBar(releasedModifiers, { keys: [{ key: "Escape" }], moreKeys: [] });
    expect(screen.getByRole("button", { name: "Esc" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Extra keys" })).not.toBeInTheDocument();
  });

  it("switches where typing goes with the direct input key, which reads as pressed while typing goes to the terminal", () => {
    const { onDirectInput, onKey } = showMobileKeyBar(releasedModifiers, defaultKeyBarLayout, true);
    const toggle = screen.getByRole("button", { name: "Type into the terminal" });
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(fireEvent.pointerDown(toggle, { cancelable: true })).toBe(false);
    fireEvent.click(toggle);
    expect(onDirectInput).toHaveBeenCalledOnce();
    expect(onKey).not.toHaveBeenCalled();
  });
});


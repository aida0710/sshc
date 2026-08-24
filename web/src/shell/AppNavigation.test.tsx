import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { NavigationResizeHandle } from "./AppNavigation";
import { maximumNavigationWidth, minimumNavigationWidth } from "./navigationLayout";

beforeEach(() => {
  vi.spyOn(window, "requestAnimationFrame").mockReturnValue(42);
  vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => undefined);
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  document.body.style.userSelect = "";
});

function renderHandle(width: number, onWidthChange = vi.fn()) {
  render(
    <LanguageProvider initial="en">
      <NavigationResizeHandle width={width} onWidthChange={onWidthChange} />
    </LanguageProvider>,
  );
  return { handle: screen.getByRole("separator", { name: "Resize navigation" }), onWidthChange };
}

describe("NavigationResizeHandle", () => {
  it("exposes its orientation and supported range", () => {
    const { handle } = renderHandle(240);
    expect(handle).toHaveAttribute("aria-orientation", "vertical");
    expect(handle).toHaveAttribute("aria-valuemin", String(minimumNavigationWidth));
    expect(handle).toHaveAttribute("aria-valuemax", String(maximumNavigationWidth));
    expect(handle).toHaveAttribute("aria-valuenow", "240");
  });

  it.each([
    ["ArrowLeft", false, 232],
    ["ArrowRight", false, 248],
    ["ArrowLeft", true, 208],
    ["ArrowRight", true, 272],
    ["Home", false, minimumNavigationWidth],
    ["End", false, maximumNavigationWidth],
  ])("handles %s with shift=%s", (key, shiftKey, expected) => {
    const { handle, onWidthChange } = renderHandle(240);
    fireEvent.keyDown(handle, { key, shiftKey });
    expect(onWidthChange).toHaveBeenCalledWith(expected);
  });

  it.each([
    [minimumNavigationWidth, "ArrowLeft", minimumNavigationWidth],
    [maximumNavigationWidth, "ArrowRight", maximumNavigationWidth],
  ])("clamps a width of %s after %s", (width, key, expected) => {
    const { handle, onWidthChange } = renderHandle(width);
    fireEvent.keyDown(handle, { key });
    expect(onWidthChange).toHaveBeenCalledWith(expected);
  });

  it("tracks a pointer and publishes the final width", () => {
    document.body.style.userSelect = "text";
    const { handle, onWidthChange } = renderHandle(240);

    fireEvent.pointerDown(handle, { button: 0, pointerId: 7, clientX: 240 });
    expect(document.body.style.userSelect).toBe("none");
    fireEvent.pointerMove(handle, { pointerId: 7, clientX: 315 });
    fireEvent.pointerUp(handle, { pointerId: 7, clientX: 315 });

    expect(onWidthChange).toHaveBeenLastCalledWith(315);
    expect(document.body.style.userSelect).toBe("text");
  });
});

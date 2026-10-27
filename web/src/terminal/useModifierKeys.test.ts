import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { modifierDoubleTapMilliseconds } from "./modifierLatch";
import { useModifierKeys } from "./useModifierKeys";

afterEach(() => { vi.restoreAllMocks(); });

function tapAt(tap: () => void, milliseconds: number) {
  vi.spyOn(performance, "now").mockReturnValue(milliseconds);
  act(tap);
}

describe("useModifierKeys", () => {
  it("applies one tap of Ctrl to the next key only", () => {
    const { result } = renderHook(() => useModifierKeys());
    tapAt(() => result.current.tap("ctrl"), 1_000);

    let first = { ctrl: false, alt: false };
    act(() => { first = result.current.takeModifiers(); });

    expect(first).toEqual({ ctrl: true, alt: false });
    expect(result.current.takeModifiers()).toEqual({ ctrl: false, alt: false });
  });

  it("keeps Ctrl for every key after a quick second tap, until it is tapped again", () => {
    const { result } = renderHook(() => useModifierKeys());
    tapAt(() => result.current.tap("ctrl"), 1_000);
    tapAt(() => result.current.tap("ctrl"), 1_000 + modifierDoubleTapMilliseconds);

    act(() => { result.current.takeModifiers(); });
    expect(result.current.modifiers.ctrl).toBe("locked");
    expect(result.current.takeModifiers()).toEqual({ ctrl: true, alt: false });

    tapAt(() => result.current.tap("ctrl"), 5_000);
    expect(result.current.modifiers.ctrl).toBe("off");
  });
});

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { saveTouchMode, touchFirstQuery, useTouchTerminal } from "./touchMode";

// The browser's answer to touchFirstQuery, which a 2-in-1 PC changes when it changes shape.
function stubTouchFirst(initially: boolean) {
  let touchFirst = initially;
  const listeners = new Set<() => void>();
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    get matches() { return query === touchFirstQuery && touchFirst; },
    media: query,
    addEventListener: (_type: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_type: string, listener: () => void) => listeners.delete(listener),
  })));
  return (next: boolean) => {
    touchFirst = next;
    listeners.forEach((listener) => listener());
  };
}

beforeEach(() => {
  fakeSettingsServer();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("useTouchTerminal", () => {
  it("follows the browser while detection is on, also when a 2-in-1 PC changes shape", () => {
    const changeShape = stubTouchFirst(true);
    const { result } = renderHook(() => useTouchTerminal());
    expect(result.current).toBe(true);

    act(() => changeShape(false));

    expect(result.current).toBe(false);
  });

  it("keeps the mouse when fixed to it, though the browser says touch first", async () => {
    stubTouchFirst(true);
    const { result } = renderHook(() => useTouchTerminal());

    await act(async () => saveTouchMode("mouse"));

    expect(result.current).toBe(false);
  });

  it("uses touch when fixed to it, though the browser says a mouse is there", async () => {
    stubTouchFirst(false);
    await act(async () => saveTouchMode("touch"));

    const { result } = renderHook(() => useTouchTerminal());

    expect(result.current).toBe(true);
  });
});

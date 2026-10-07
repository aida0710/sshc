import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defaultStack, fontStack } from "./fonts";
import { useTerminalFont } from "./useTerminalFont";

const originalFonts = Object.getOwnPropertyDescriptor(document, "fonts");
afterEach(() => {
  if (originalFonts === undefined) Reflect.deleteProperty(document, "fonts");
  else Object.defineProperty(document, "fonts", originalFonts);
});

function deferredFont() {
  let resolve!: () => void;
  let reject!: () => void;
  const completion = new Promise<FontFace[]>((done, fail) => {
    resolve = () => done([]);
    reject = () => fail(new Error("font unavailable"));
  });
  const load = vi.fn(() => completion);
  Object.defineProperty(document, "fonts", {
    configurable: true,
    value: { load, check: vi.fn(() => false) },
  });
  return { load, resolve, reject };
}

describe("useTerminalFont", () => {
  it("keeps the fallback until the selected font is available", async () => {
    const font = deferredFont();
    const { result } = renderHook(() => useTerminalFont("jetbrains-mono", 13));
    expect(result.current).toBe(defaultStack);
    expect(font.load).toHaveBeenCalledWith(`13px ${fontStack("jetbrains-mono")}`);
    await act(async () => { font.resolve(); });
    expect(result.current).toBe(fontStack("jetbrains-mono"));
  });

  it("uses an already loaded font on the first render", () => {
    deferredFont();
    vi.mocked(document.fonts.check).mockReturnValue(true);
    const { result } = renderHook(() => useTerminalFont("jetbrains-mono", 13));
    expect(result.current).toBe(fontStack("jetbrains-mono"));
  });

  it("does not restore an old selection when its font finishes loading", async () => {
    const font = deferredFont();
    const { result, rerender } = renderHook(({ name }) => useTerminalFont(name, 13), { initialProps: { name: "jetbrains-mono" } });
    rerender({ name: "" });
    await act(async () => { font.resolve(); });
    expect(result.current).toBe(defaultStack);
  });

  it("keeps a usable fallback when loading fails", async () => {
    const font = deferredFont();
    const { result } = renderHook(() => useTerminalFont("jetbrains-mono", 13));
    await act(async () => { font.reject(); });
    await waitFor(() => expect(result.current).toBe(defaultStack));
  });

  it("uses the system font without requesting a web font", () => {
    const font = deferredFont();
    const { result } = renderHook(() => useTerminalFont("", 13));
    expect(result.current).toBe(defaultStack);
    expect(font.load).not.toHaveBeenCalled();
  });
});

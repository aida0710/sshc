import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useAppViewport } from "./useAppViewport";

afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = ""; });

it("fits the visual viewport and restores navigation after the keyboard closes", () => {
  const viewport = Object.assign(new EventTarget(), { height: 700, offsetTop: 0, scale: 1 });
  vi.stubGlobal("visualViewport", viewport);
  vi.stubGlobal("innerHeight", 700);
  const { unmount } = renderHook(useAppViewport);
  const input = document.createElement("input");
  document.body.append(input);
  act(() => {
    input.focus();
    viewport.height = 360;
    viewport.offsetTop = 12;
    viewport.dispatchEvent(new Event("resize"));
  });
  expect(document.documentElement.style.getPropertyValue("--app-viewport-height")).toBe("360px");
  expect(document.documentElement.style.getPropertyValue("--app-viewport-top")).toBe("12px");
  expect(document.documentElement.dataset.keyboardOpen).toBe("true");
  act(() => {
    viewport.height = 700;
    viewport.offsetTop = 0;
    viewport.dispatchEvent(new Event("resize"));
  });
  expect(document.documentElement.dataset.keyboardOpen).toBe("false");
  unmount();
  expect(document.documentElement.style.getPropertyValue("--app-viewport-height")).toBe("");
});

it("does not refit the terminal grid while pinch zoom is active", () => {
  const viewport = Object.assign(new EventTarget(), { height: 700, offsetTop: 0, scale: 1 });
  vi.stubGlobal("visualViewport", viewport);
  const { unmount } = renderHook(useAppViewport);
  act(() => {
    viewport.scale = 2;
    viewport.height = 350;
    viewport.dispatchEvent(new Event("resize"));
  });
  expect(document.documentElement.style.getPropertyValue("--app-viewport-height")).toBe("700px");
  unmount();
});

it("retains the pre-IME height while moving between fields in a resized WebView", async () => {
  const viewport = Object.assign(new EventTarget(), { height: 700, offsetTop: 0, scale: 1 });
  vi.stubGlobal("visualViewport", viewport);
  vi.stubGlobal("innerHeight", 700);
  const { unmount } = renderHook(useAppViewport);
  const first = document.createElement("input");
  const second = document.createElement("input");
  document.body.append(first, second);
  act(() => {
    first.focus();
    vi.stubGlobal("innerHeight", 360);
    viewport.height = 360;
    viewport.dispatchEvent(new Event("resize"));
  });
  expect(document.documentElement.dataset.keyboardOpen).toBe("true");
  await act(async () => { second.focus(); });
  expect(document.documentElement.dataset.keyboardOpen).toBe("true");
  expect(document.documentElement.style.getPropertyValue("--app-viewport-height")).toBe("360px");
  unmount();
});

it("reveals the focused form field after keyboard layout but leaves terminal positioning alone", () => {
  const viewport = Object.assign(new EventTarget(), { height: 700, offsetTop: 0, scale: 1 });
  vi.stubGlobal("visualViewport", viewport);
  vi.stubGlobal("innerHeight", 700);
  let frame: FrameRequestCallback | undefined;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => { frame = callback; return 1; });
  vi.stubGlobal("cancelAnimationFrame", () => { frame = undefined; });
  const { unmount } = renderHook(useAppViewport);
  const input = document.createElement("input");
  input.scrollIntoView = vi.fn();
  document.body.append(input);
  act(() => {
    input.focus();
    viewport.height = 360;
    viewport.dispatchEvent(new Event("resize"));
  });
  act(() => { frame?.(0); });
  expect(input.scrollIntoView).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
  const terminal = document.createElement("div");
  terminal.dataset.terminalHost = "";
  const text = document.createElement("textarea");
  text.scrollIntoView = vi.fn();
  terminal.append(text);
  document.body.append(terminal);
  act(() => { text.focus(); viewport.dispatchEvent(new Event("resize")); });
  act(() => { frame?.(0); });
  expect(text.scrollIntoView).not.toHaveBeenCalled();
  unmount();
});

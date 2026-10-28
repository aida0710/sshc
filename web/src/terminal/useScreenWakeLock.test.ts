import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useScreenWakeLock } from "./useScreenWakeLock";

type FakeLock = { released: boolean; release: () => Promise<void> };

// installWakeLock stands in for navigator.wakeLock, which jsdom does not have.
function installWakeLock() {
  const locks: FakeLock[] = [];
  const request = vi.fn(async () => {
    const lock: FakeLock = { released: false, release: vi.fn(async () => { lock.released = true; }) };
    locks.push(lock);
    return lock;
  });
  Object.defineProperty(navigator, "wakeLock", { configurable: true, value: { request } });
  return { request, locks };
}

let visibility: DocumentVisibilityState = "visible";

beforeEach(() => {
  visibility = "visible";
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "wakeLock");
  vi.restoreAllMocks();
});

async function showPage(state: DocumentVisibilityState) {
  visibility = state;
  await act(async () => { document.dispatchEvent(new Event("visibilitychange")); });
}

describe("useScreenWakeLock", () => {
  it("holds the screen on while active and lets go when no longer active", async () => {
    const wakeLock = installWakeLock();
    const { rerender } = renderHook(({ active }) => useScreenWakeLock(active), { initialProps: { active: true } });
    await act(async () => {});

    expect(wakeLock.request).toHaveBeenCalledWith("screen");
    rerender({ active: false });
    expect(wakeLock.locks[0]!.released).toBe(true);
  });

  it("asks again when the page is shown after the browser let go of the lock", async () => {
    const wakeLock = installWakeLock();
    renderHook(() => useScreenWakeLock(true));
    await act(async () => {});

    await showPage("hidden");
    wakeLock.locks[0]!.released = true;
    expect(wakeLock.request).toHaveBeenCalledTimes(1);
    await showPage("visible");

    expect(wakeLock.request).toHaveBeenCalledTimes(2);
  });

  it("does not ask twice while it still holds the lock", async () => {
    const wakeLock = installWakeLock();
    renderHook(() => useScreenWakeLock(true));
    await act(async () => {});

    await showPage("visible");

    expect(wakeLock.request).toHaveBeenCalledTimes(1);
  });

  it("asks for nothing while inactive", async () => {
    const wakeLock = installWakeLock();
    renderHook(() => useScreenWakeLock(false));
    await act(async () => {});

    expect(wakeLock.request).not.toHaveBeenCalled();
  });

  it("leaves the screen alone when the browser refuses the lock", async () => {
    const request = vi.fn(async () => { throw new DOMException("battery saver", "NotAllowedError"); });
    Object.defineProperty(navigator, "wakeLock", { configurable: true, value: { request } });
    const { unmount } = renderHook(() => useScreenWakeLock(true));
    await act(async () => {});

    expect(request).toHaveBeenCalledOnce();
    unmount();
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { whenPageResumes } from "./pageResume";

function setVisibility(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => state });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  setVisibility("visible");
});

afterEach(() => {
  vi.useRealTimers();
});

describe("whenPageResumes", () => {
  it("calls back when the page comes back from the back-forward cache, but not on its first showing", () => {
    const resumed = vi.fn();
    const stop = whenPageResumes(resumed);

    window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: false }));
    expect(resumed).not.toHaveBeenCalled();
    window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
    expect(resumed).toHaveBeenCalledOnce();
    stop();
  });

  it("calls back when the network comes back", () => {
    const resumed = vi.fn();
    const stop = whenPageResumes(resumed);

    window.dispatchEvent(new Event("online"));

    expect(resumed).toHaveBeenCalledOnce();
    stop();
  });

  it("tells a wake from sleep by a check that runs far later than it was due", () => {
    const resumed = vi.fn();
    const stop = whenPageResumes(resumed);

    vi.advanceTimersByTime(60_000);
    expect(resumed).not.toHaveBeenCalled();

    // The computer slept for ten minutes: the clock moved on without the timer running.
    vi.setSystemTime(Date.now() + 10 * 60_000);
    vi.advanceTimersByTime(5_000);
    expect(resumed).toHaveBeenCalledOnce();
    stop();
  });

  it("counts a page hidden for long as away when it is shown again, and a short look elsewhere as not", () => {
    const resumed = vi.fn();
    const stop = whenPageResumes(resumed);

    setVisibility("hidden");
    vi.setSystemTime(Date.now() + 5_000);
    setVisibility("visible");
    expect(resumed).not.toHaveBeenCalled();

    setVisibility("hidden");
    vi.setSystemTime(Date.now() + 5 * 60_000);
    setVisibility("visible");
    expect(resumed).toHaveBeenCalledOnce();
    stop();
  });

  it("stops listening once stopped", () => {
    const resumed = vi.fn();
    whenPageResumes(resumed)();

    window.dispatchEvent(new Event("online"));
    vi.setSystemTime(Date.now() + 10 * 60_000);
    vi.advanceTimersByTime(5_000);

    expect(resumed).not.toHaveBeenCalled();
  });
});

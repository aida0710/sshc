import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { runArt } from "./artAnimation";

const drawn = vi.hoisted(() => ({ moments: [] as number[] }));

vi.mock("./emptyTerminalArt", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./emptyTerminalArt")>()),
  createArt: () => (seconds: number) => drawn.moments.push(seconds),
}));

function laidOutCanvas(): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  Object.defineProperty(canvas, "clientWidth", { value: 400 });
  Object.defineProperty(canvas, "clientHeight", { value: 200 });
  return canvas;
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", { configurable: true, value: hidden });
  document.dispatchEvent(new Event("visibilitychange"));
}

const options = { labels: [], globe: { x: 0.5, y: 0.5, radius: 0.4 }, zoom: 1, fish: [], fishLabels: false, swimDepth: 1 };

beforeEach(() => {
  drawn.moments = [];
  vi.useFakeTimers();
});

afterEach(() => {
  setHidden(false);
  vi.useRealTimers();
});

describe("runArt", () => {
  it("draws one still frame when the art does not move", async () => {
    const stop = runArt(laidOutCanvas(), { art: "network", fontSize: 13, intensity: 1, options, moving: false, startAt: 6 });
    await vi.advanceTimersByTimeAsync(5000);

    expect(drawn.moments).toEqual([6]);
    stop();
  });

  it("keeps drawing while the page is shown, pauses while it is hidden, and goes on from where it stopped", async () => {
    const stop = runArt(laidOutCanvas(), { art: "network", fontSize: 13, intensity: 1, options, moving: true, startAt: 6 });
    await vi.advanceTimersByTimeAsync(1000);
    const whileShown = drawn.moments.length;
    expect(whileShown).toBeGreaterThanOrEqual(12);

    setHidden(true);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(drawn.moments).toHaveLength(whileShown);

    setHidden(false);
    await vi.advanceTimersByTimeAsync(0);
    // The hidden ten seconds do not count on the art's clock.
    expect(drawn.moments.at(-1)).toBeLessThan(6 + 1.5);
    stop();
  });

  it("stops drawing once stopped", async () => {
    const stop = runArt(laidOutCanvas(), { art: "globe", fontSize: 13, intensity: 1, options, moving: true, startAt: 6 });
    await vi.advanceTimersByTimeAsync(500);
    stop();
    const atStop = drawn.moments.length;
    await vi.advanceTimersByTimeAsync(5000);

    expect(drawn.moments).toHaveLength(atStop);
  });
});

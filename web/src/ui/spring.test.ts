import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { runSpring } from "./spring";

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
});

afterEach(() => {
  vi.useRealTimers();
});

function positionsOf(velocity: number) {
  const positions: number[] = [];
  const onRest = vi.fn();
  runSpring({ from: -200, to: 0, velocity, onFrame: (position) => positions.push(position), onRest });
  vi.advanceTimersByTime(2000);
  return { positions, onRest };
}

describe("runSpring", () => {
  it("comes to rest on its target without passing it after a slow let-go", () => {
    const { positions, onRest } = positionsOf(0);
    expect(onRest).toHaveBeenCalledOnce();
    expect(positions.at(-1)).toBe(0);
    expect(Math.max(...positions)).toBeLessThanOrEqual(0);
  });

  it("leaves at the speed the finger let go with, so a fling gets further sooner", () => {
    const slow = positionsOf(0).positions;
    const flung = positionsOf(3).positions;
    expect(flung[2]).toBeGreaterThan(slow[2]!);
  });

  it("stops where it is when a finger catches it", () => {
    const onFrame = vi.fn();
    const onRest = vi.fn();
    const run = runSpring({ from: -200, to: 0, velocity: 0, onFrame, onRest });
    vi.advanceTimersByTime(64);
    const caught = run.stop();
    vi.advanceTimersByTime(2000);
    expect(caught).toBeGreaterThan(-200);
    expect(caught).toBeLessThan(0);
    expect(onFrame).toHaveBeenLastCalledWith(caught);
    expect(onRest).not.toHaveBeenCalled();
  });

  it("lands at once where motion is reduced", () => {
    const onFrame = vi.fn();
    const onRest = vi.fn();
    runSpring({ from: -200, to: 0, velocity: 3, onFrame, onRest, reducedMotion: true });
    expect(onFrame).toHaveBeenCalledExactlyOnceWith(0);
    expect(onRest).toHaveBeenCalledOnce();
  });
});

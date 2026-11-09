import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { runSpring, runSprings } from "./spring";

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

  it("goes the same way to rest on slow frames as on quick ones, without shaking", () => {
    const pathOn = (frameMilliseconds: number) => {
      // Frames of a given length, on timers.
      vi.useRealTimers();
      vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] });
      const positions: number[] = [];
      runSpring({ from: 80, to: 0, velocity: 4.5, onFrame: (position) => positions.push(position), onRest: () => undefined, clock: {
        now: () => performance.now(),
        requestFrame: (callback) => window.setTimeout(() => callback(performance.now()), frameMilliseconds),
        cancelFrame: (id) => window.clearTimeout(id),
      } });
      vi.advanceTimersByTime(250);
      const at250 = positions.at(-1)!;
      vi.advanceTimersByTime(2000);
      return { at250, positions };
    };
    const quick = pathOn(8);
    const slow = pathOn(125);
    expect(slow.at250).toBeCloseTo(quick.at250, 0);
    // Out past where it started, then back: once it turns, it only comes nearer.
    const turned = slow.positions.findIndex((position, index) => index > 0 && position < slow.positions[index - 1]!);
    const back = slow.positions.slice(turned);
    expect(back.every((position, index) => index === 0 || position <= back[index - 1]!)).toBe(true);
    expect(Math.min(...slow.positions)).toBeGreaterThanOrEqual(0);
  });

  it("asks for no more frames once stopped from its own frame", () => {
    const onFrame = vi.fn();
    const onRest = vi.fn();
    const run = runSpring({ from: -200, to: 0, velocity: 0, onFrame: (position) => {
      onFrame(position);
      if (position > -150) run.stop();
    }, onRest });
    vi.advanceTimersByTime(2000);
    const stoppedAt = onFrame.mock.calls.length;
    vi.advanceTimersByTime(2000);
    expect(onFrame).toHaveBeenCalledTimes(stoppedAt);
    expect(onFrame.mock.lastCall![0]).toBeLessThan(0);
    expect(onRest).not.toHaveBeenCalled();
  });

  it("lands at once where motion is reduced", () => {
    const onFrame = vi.fn();
    const onRest = vi.fn();
    runSpring({ from: -200, to: 0, velocity: 3, onFrame, onRest, reducedMotion: true });
    expect(onFrame).toHaveBeenCalledExactlyOnceWith(0);
    expect(onRest).toHaveBeenCalledOnce();
  });

  it("moves several values together and rests once all of them are there", () => {
    const frames: number[][] = [];
    const onRest = vi.fn();
    runSprings({ from: [1, 0, 0], to: [2, 50, -30], velocity: [0, 0, 0], scale: [100, 1, 1], onFrame: (values) => frames.push(values), onRest });
    vi.advanceTimersByTime(2000);
    expect(onRest).toHaveBeenCalledOnce();
    expect(frames.at(-1)).toEqual([2, 50, -30]);
  });
});

import { describe, expect, it } from "vitest";
import { newVelocityTracker } from "./pointerVelocity";

describe("newVelocityTracker", () => {
  it("follows the speed of the last moves, weighted toward the latest", () => {
    const tracker = newVelocityTracker(0, 0, 0);
    tracker.move(10, 0, 10);
    tracker.move(40, 20, 20);
    const speed = tracker.velocity(20);
    expect(speed.x).toBeGreaterThan(1);
    expect(speed.x).toBeLessThan(3);
    expect(speed.y).toBeGreaterThan(0);
  });

  it("has no speed once the pointer rested before letting go", () => {
    const tracker = newVelocityTracker(0, 0, 0);
    tracker.move(100, 0, 10);
    expect(tracker.velocity(200)).toEqual({ x: 0, y: 0 });
  });
});

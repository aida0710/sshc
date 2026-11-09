import { describe, expect, it } from "vitest";
import { rubberBand, rubberBandSlope } from "./rubberBand";

describe("rubberBand", () => {
  it("moves less than the finger, and less and less the further it pulls, both ways", () => {
    const near = rubberBand(50, 400);
    const far = rubberBand(500, 400);
    expect(near).toBeGreaterThan(0);
    expect(near).toBeLessThan(50);
    expect(far - near).toBeLessThan(500 - 50);
    expect(far).toBeLessThan(400);
    expect(rubberBand(-50, 400)).toBeCloseTo(-near);
  });

  it("says how far what it draws moves for each pixel the finger moves, less the further it pulls", () => {
    const at = (overflow: number) => rubberBand(overflow + 0.01, 400) - rubberBand(overflow - 0.01, 400);
    expect(rubberBandSlope(120, 400)).toBeCloseTo(at(120) / 0.02, 3);
    expect(rubberBandSlope(-120, 400)).toBeCloseTo(at(-120) / 0.02, 3);
    expect(rubberBandSlope(300, 400)).toBeLessThan(rubberBandSlope(30, 400));
  });
});

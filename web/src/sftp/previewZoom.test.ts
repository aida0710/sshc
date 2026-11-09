import { describe, expect, it } from "vitest";
import { fittedZoom, maxPreviewScale, movedBy, previewZoomStep, stretchedFollow, stretchedZoom, videoViewBox, zoomedAround } from "./previewZoom";

const size = { width: 400, height: 300 };

describe("zoomedAround", () => {
  it("keeps the spot under the pointer where it is", () => {
    const zoomed = zoomedAround(fittedZoom, 2, { x: 100, y: -50 }, size);
    expect(zoomed).toEqual({ scale: 2, x: -100, y: 50 });
    // The picture's spot drawn under the pointer is the one that was there: (pointer - x) / scale.
    expect((100 - zoomed.x) / zoomed.scale).toBe(100);
    expect((-50 - zoomed.y) / zoomed.scale).toBe(-50);
  });

  it("stays between the fitted picture and eight times its size", () => {
    expect(zoomedAround(fittedZoom, 0.5, { x: 0, y: 0 }, size)).toEqual(fittedZoom);
    expect(zoomedAround({ scale: 7, x: 0, y: 0 }, 4, { x: 0, y: 0 }, size).scale).toBe(maxPreviewScale);
  });

  it("goes back to the fitted picture, centred, after zooming out all the way", () => {
    const zoomed = zoomedAround(fittedZoom, 4, { x: 150, y: 100 }, size);
    expect(zoomedAround(zoomed, 1 / 4, { x: -150, y: 0 }, size)).toEqual({ scale: 1, x: 0, y: 0 });
  });

  it("doubles in four presses", () => {
    expect(previewZoomStep ** 4).toBeCloseTo(2, 10);
  });
});

describe("movedBy", () => {
  it("stops at the picture's edges", () => {
    const zoomed = { scale: 2, x: 0, y: 0 };
    expect(movedBy(zoomed, 1000, -1000, size)).toEqual({ scale: 2, x: 200, y: -150 });
  });

  it("does not move a fitted picture", () => {
    expect(movedBy(fittedZoom, 30, 30, size)).toEqual(fittedZoom);
  });
});

describe("stretchedFollow", () => {
  const size = { width: 400, height: 300 };

  it("follows the finger fully within the edges, and as stretchedZoom draws past them", () => {
    expect(stretchedFollow({ scale: 2, x: 50, y: -50 }, size)).toEqual({ x: 1, y: 1 });
    // Twice the size reaches 200px across; 260px is 60px past the edge.
    const past = stretchedFollow({ scale: 2, x: 260, y: 0 }, size);
    const drawnAt = (x: number) => stretchedZoom({ scale: 2, x, y: 0 }, size).x;
    expect(past.x).toBeCloseTo((drawnAt(260.01) - drawnAt(259.99)) / 0.02, 3);
    expect(past.x).toBeLessThan(1);
    expect(past.y).toBe(1);
  });
});

describe("videoViewBox", () => {
  it("shows the middle of the frame when zoomed around its centre", () => {
    expect(videoViewBox({ scale: 2, x: 0, y: 0 }, size)).toBe("inset(25.000% 25.000% 25.000% 25.000%)");
  });

  it("shows the left edge of the frame when moved all the way right", () => {
    expect(videoViewBox({ scale: 2, x: 200, y: 0 }, size)).toBe("inset(25.000% 50.000% 25.000% 0.000%)");
  });

  it("draws nothing for a fitted video", () => {
    expect(videoViewBox(fittedZoom, size)).toBe("none");
  });
});

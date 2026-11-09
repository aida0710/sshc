import { describe, expect, it } from "vitest";
import { releasedSwipe, swipeAxisOf, type PreviewSwipe } from "./previewSwipe";

const everything = () => true;
const still = { x: 0, y: 0 };

describe("swipeAxisOf", () => {
  it("waits past the wobble of a tap, then picks the way the stroke went most", () => {
    expect(swipeAxisOf({ x: 4, y: 3 })).toBeNull();
    expect(swipeAxisOf({ x: -20, y: 8 })).toBe("across");
    expect(swipeAxisOf({ x: 6, y: 20 })).toBe("down");
    expect(swipeAxisOf({ x: 6, y: -20 })).toBe("down");
  });
});

describe("releasedSwipe", () => {
  it("steps once the picture went a quarter of its width, or was flicked that way", () => {
    expect(releasedSwipe({ axis: "across", offset: { x: -120, y: 0 }, velocity: still, width: 400, allows: everything })).toBe("next");
    expect(releasedSwipe({ axis: "across", offset: { x: 120, y: 0 }, velocity: still, width: 400, allows: everything })).toBe("previous");
    expect(releasedSwipe({ axis: "across", offset: { x: -40, y: 0 }, velocity: still, width: 400, allows: everything })).toBeNull();
    expect(releasedSwipe({ axis: "across", offset: { x: -40, y: 0 }, velocity: { x: -1.2, y: 0 }, width: 400, allows: everything })).toBe("next");
  });

  it("does not take a flick back against the way the picture went", () => {
    expect(releasedSwipe({ axis: "across", offset: { x: -40, y: 0 }, velocity: { x: 1.2, y: 0 }, width: 400, allows: everything })).toBeNull();
  });

  it("closes once pulled far enough down, or flicked down", () => {
    expect(releasedSwipe({ axis: "down", offset: { x: 0, y: 120 }, velocity: still, width: 400, allows: everything })).toBe("close");
    expect(releasedSwipe({ axis: "down", offset: { x: 0, y: 30 }, velocity: { x: 0, y: 1 }, width: 400, allows: everything })).toBe("close");
    expect(releasedSwipe({ axis: "down", offset: { x: 0, y: -120 }, velocity: { x: 0, y: -1 }, width: 400, allows: everything })).toBeNull();
  });

  it("goes nowhere there is no file to go to", () => {
    const allows = (swipe: PreviewSwipe) => swipe !== "next";
    expect(releasedSwipe({ axis: "across", offset: { x: -300, y: 0 }, velocity: { x: -2, y: 0 }, width: 400, allows })).toBeNull();
  });
});

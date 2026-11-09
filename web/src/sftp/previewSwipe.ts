import type { PointerVelocity } from "../ui/pointerVelocity";

// A finger's stroke across a fitted picture in a file preview, as in a phone's photos: the picture follows the finger,
// and let go, to the right it shows the previous file, to the left the next, and down it closes the preview.
export type PreviewSwipe = "previous" | "next" | "close";

// What the preview does with a swipe, and whether it has a file to go to that way.
export type PreviewSwipeHandler = {
  allows(swipe: PreviewSwipe): boolean;
  swipe(swipe: PreviewSwipe): void;
};

type Point = { x: number; y: number };

// The way a stroke moves the picture, decided once (swipeAxisOf): across, to step, or up and down, to close.
export type SwipeAxis = "across" | "down";

// swipeAxisThreshold is how far a finger travels before its stroke picks a way: past the wobble of a tap or a double
// tap, so neither moves the picture.
const swipeAxisThreshold = 8;
// A stroke let go past this share of the picture's width steps to the next or previous file.
const stepShare = 0.25;
// A stroke let go this far down closes the preview.
const closeDistance = 96;
// A flick this fast (pixels a millisecond) the way of its stroke steps or closes however short it was.
const flickSpeed = 0.5;

// swipeAxisOf is the way a stroke moved from its start, once it has gone far enough to tell.
export function swipeAxisOf(moved: Point): SwipeAxis | null {
  if (Math.hypot(moved.x, moved.y) < swipeAxisThreshold) return null;
  return Math.abs(moved.x) >= Math.abs(moved.y) ? "across" : "down";
}

// releasedSwipe is what a stroke let go with offset and velocity does: a step or a close when it went far enough or
// was flicked that way, and nothing (the picture goes back) otherwise.
export function releasedSwipe({ axis, offset, velocity, width, allows }: {
  axis: SwipeAxis;
  offset: Point;
  velocity: PointerVelocity;
  width: number;
  allows: (swipe: PreviewSwipe) => boolean;
}): PreviewSwipe | null {
  if (axis === "down") {
    const closes = offset.y > closeDistance || (velocity.y > flickSpeed && offset.y > 0);
    return closes && allows("close") ? "close" : null;
  }
  const swipe: PreviewSwipe = offset.x < 0 ? "next" : "previous";
  const far = Math.abs(offset.x) > width * stepShare;
  const flicked = Math.abs(velocity.x) > flickSpeed && Math.sign(velocity.x) === Math.sign(offset.x);
  return (far || flicked) && allows(swipe) ? swipe : null;
}

// A finger's swipe across a fitted picture in a file preview, as in a phone's photos: to the right shows the
// previous file, to the left the next, and down closes the preview.
export type PreviewSwipe = "previous" | "next" | "close";

type Point = { x: number; y: number };

// swipeDistance is how far a finger travels before its stroke counts as a swipe: well past the wobble of a tap
// or a double tap, and still a short flick of the thumb.
const swipeDistance = 56;

// swipeDominance is how many times longer a swipe runs along its direction than across it, so that a
// diagonal stroke does nothing rather than a guess.
const swipeDominance = 2;

// swipeOf reads a stroke from where the finger was put down to where it was lifted.
export function swipeOf(from: Point, to: Point): PreviewSwipe | null {
  const across = to.x - from.x;
  const down = to.y - from.y;
  if (Math.abs(across) >= swipeDistance && Math.abs(across) >= swipeDominance * Math.abs(down)) return across > 0 ? "previous" : "next";
  if (down >= swipeDistance && down >= swipeDominance * Math.abs(across)) return "close";
  return null;
}

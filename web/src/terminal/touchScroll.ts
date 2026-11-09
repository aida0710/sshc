import { rubberBand, rubberBandSlope, unrubberBand } from "../ui/rubberBand";
import { runSpring, type SpringRun } from "../ui/spring";
import type { ScreenPoint } from "./tappedLink";

export type Scroller = { scrollLines(amount: number): void };
// Finger is where the finger is now and what it first touched. Coasting after the finger lifts keeps its last place.
export type Finger = ScreenPoint & { touched: EventTarget | null };
// TouchScrollTarget is what a finger scrolls. It is told about the finger because a program that reports the mouse
// wheel is told where the wheel turned.
export type TouchScrollTarget = {
  scrollLines(amount: number, finger: Finger): void;
  // smoothScrollback is the scrollback, when that is what this finger moves; null while a program scrolls its own text,
  // which moves a row at a time however the finger moves.
  smoothScrollback?(finger: Finger): SmoothScrollback | null;
};
// SmoothScrollback is scrollback a finger moves by the pixel, as a page moves in a browser, rather than a row at a time:
// while the finger is on it, its rows are drawn moved by part of a row, and they settle on a row once it stops.
export type SmoothScrollback = {
  // room is how many rows it can move toward older lines (above) and newer lines (below).
  room(): { above: number; below: number };
  // show draws the rows moved toward newer lines by a fraction of a row (0 ≤ fraction < 1).
  show(fraction: number): void;
  // overscroll draws the rows pulled past an end by pixels: past the newest line when positive, past the oldest when
  // negative, with nothing beyond them. height is how tall the rows shown are, which a pull past an end is measured
  // against (rubberBand).
  overscroll(pixels: number): void;
  height(): number;
};
// The time a scrollback left between two rows takes to settle on the nearer one.
const settleMilliseconds = 120;
// After the finger lets go, the speed falls by e in this time, nearer to how a page flung on a phone slows down (iOS's
// normal deceleration, 0.998 a millisecond, is about 500 ms). At 180 ms, as before, a flick stopped soon after the
// finger let go, which made the text feel stiff.
const coastTimeConstantMs = 350;
// The fastest a flick moves the text, in pixels a millisecond: Android's default maximum fling, 8,000 dp a second.
// At 3, as before, a fast flick went no farther than a slow one.
const maximumFlickPixelsPerMs = 8;
// A flick that coasts into an end bounces off it with this share of the speed it reached it with: enough to show it
// hit the end, not so much that the text leaves the screen.
const bounceShare = 0.5;
type TouchScrollOptions = {
  now?: () => number;
  requestFrame?: (callback: FrameRequestCallback) => number;
  cancelFrame?: (id: number) => void;
  canScroll?: () => boolean;
  reducedMotion?: () => boolean;
};

export function newTouchScroll(view: Scroller, cellHeight: () => number, options: TouchScrollOptions = {}) {
  const now = options.now ?? (() => performance.now());
  const requestFrame = options.requestFrame ?? ((callback) => requestAnimationFrame(callback));
  const cancelFrame = options.cancelFrame ?? ((id) => cancelAnimationFrame(id));
  const canScroll = options.canScroll ?? (() => true);
  let last = 0;
  let lastTime = 0;
  let carried = 0;
  let velocity = 0;
  let active = false;
  let frame: number | null = null;
  // smooth is the scrollback the current gesture moves by the pixel, or null; fraction is how far its rows are drawn
  // past the row shown at the top, toward newer lines.
  let smooth: SmoothScrollback | null = null;
  let fraction = 0;
  // pull is how far a finger has gone past an end of the scrollback, in pixels (positive past the newest line), and
  // pulled how far the rows are drawn past it: less than the finger, as a band gives (rubberBand). bounce is the spring
  // that takes the rows back to the end once the finger lets go, or after a flick hit it.
  let pull = 0;
  let pulled = 0;
  let bounce: SpringRun | null = null;
  const clock = { now, requestFrame, cancelFrame };
  const drawPulled = (pixels: number) => {
    pulled = pixels;
    smooth?.overscroll(pixels);
  };
  const stopBounce = () => {
    bounce?.stop();
    bounce = null;
  };
  // bounceBack runs the rows from where they are drawn past an end back to it, leaving at speed (pixels a
  // millisecond, toward newer lines).
  const bounceBack = (speed: number) => {
    stopBounce();
    pull = 0;
    bounce = runSpring({
      from: pulled,
      to: 0,
      velocity: speed,
      onFrame: drawPulled,
      onRest: () => {
        bounce = null;
      },
      reducedMotion: options.reducedMotion?.() === true,
      clock,
    });
  };

  // stop ends the coasting or settling, and leaves the rows where they are drawn.
  const stop = () => {
    if (frame !== null) cancelFrame(frame);
    frame = null;
    active = false;
    velocity = 0;
    carried = 0;
  };
  const showFraction = (next: number) => {
    fraction = next;
    smooth?.show(next);
  };
  // cancel ends the gesture and puts the rows back on a row, at their end.
  const cancel = () => {
    stop();
    stopBounce();
    pull = 0;
    if (pulled !== 0) drawPulled(0);
    if (fraction !== 0) showFraction(0);
  };
  // moveSmoothly moves the scrollback by rows, whole rows through the scrollback and the rest by drawing the rows
  // moved. A finger that goes on past the newest or the oldest row pulls the rows past it, less and less the further
  // it goes; text that coasts into an end bounces off it.
  const moveSmoothly = (scrollback: SmoothScrollback, rows: number) => {
    const cell = cellHeight();
    let left = rows;
    // A finger past an end first takes back its pull before the text moves again.
    if (pull !== 0) {
      const remaining = pull + left * cell;
      if (Math.sign(remaining) === Math.sign(pull)) {
        pull = remaining;
        drawPulled(rubberBand(pull, scrollback.height()));
        return 0;
      }
      pull = 0;
      drawPulled(0);
      left = remaining / cell;
    }
    const room = scrollback.room();
    const total = fraction + left;
    let lines = Math.floor(total);
    let next = total - lines;
    const pastNewest = lines >= room.below;
    const pastOldest = lines < -room.above;
    const end = pastNewest ? room.below : -room.above;
    if (pastNewest || pastOldest) {
      lines = end;
      next = 0;
    }
    if (lines !== 0) view.scrollLines(lines);
    showFraction(next);
    if (!pastNewest && !pastOldest) return lines;
    const beyond = (total - end) * cell;
    if (active) {
      // The finger's speed is kept: let go past the end, the rows leave it at that speed (end).
      pull = beyond;
      drawPulled(rubberBand(pull, scrollback.height()));
      return lines;
    }
    // There is nothing left to coast through: the text bounces off the end.
    if (velocity !== 0) bounceBack(velocity * bounceShare);
    velocity = 0;
    return lines;
  };
  const movePixels = (pixels: number) => {
    const cell = cellHeight();
    if (cell <= 0) return 0;
    if (smooth !== null) return moveSmoothly(smooth, pixels / cell);
    carried += pixels / cell;
    const lines = Math.trunc(carried);
    if (lines !== 0) {
      carried -= lines;
      view.scrollLines(lines);
    }
    return lines;
  };
  // settle moves rows left between two rows onto the nearer one, quickly and slowing down, as a page in a browser
  // comes to rest; at once where motion is reduced.
  const settle = () => {
    frame = null;
    if (smooth === null || fraction === 0) return;
    const from = fraction;
    const onto = fraction >= 0.5 ? 1 : 0;
    const land = () => {
      frame = null;
      if (onto === 1) view.scrollLines(1);
      showFraction(0);
    };
    if (options.reducedMotion?.() === true) {
      land();
      return;
    }
    const began = now();
    const step = () => {
      frame = null;
      const progress = Math.min(1, (now() - began) / settleMilliseconds);
      if (progress >= 1) {
        land();
        return;
      }
      const eased = 1 - (1 - progress) ** 3;
      showFraction(from + (onto - from) * eased);
      frame = requestFrame(step);
    };
    frame = requestFrame(step);
  };
  const coast = () => {
    frame = null;
    if (!canScroll() || options.reducedMotion?.() === true) {
      cancel();
      return;
    }
    const time = now();
    const elapsed = time - lastTime;
    // Do not jump through scrollback after a suspended/backgrounded frame.
    if (elapsed > 100 || elapsed < 0) {
      cancel();
      return;
    }
    lastTime = time;
    movePixels(velocity * elapsed);
    velocity *= Math.exp(-elapsed / coastTimeConstantMs);
    if (Math.abs(velocity) >= 0.02) frame = requestFrame(coast);
    else settle();
  };

  return {
    // start begins a gesture. smoothScrollback is what it moves by the pixel, if anything; a finger that lands on rows
    // still settling or coasting takes them where they are drawn.
    start(y: number, smoothScrollback: SmoothScrollback | null = null) {
      if (smoothScrollback !== smooth) cancel();
      stop();
      // Rows bouncing back from an end are caught where they are drawn.
      if (bounce !== null && smoothScrollback !== null) {
        stopBounce();
        pull = unrubberBand(pulled, smoothScrollback.height());
      }
      smooth = smoothScrollback;
      active = canScroll();
      last = y;
      lastTime = now();
    },
    move(y: number): number {
      if (!active || !canScroll()) {
        cancel();
        return 0;
      }
      const time = now();
      const elapsed = time - lastTime;
      const pixels = last - y;
      if (elapsed > 0) {
        const nextVelocity = Math.max(-maximumFlickPixelsPerMs, Math.min(maximumFlickPixelsPerMs, pixels / Math.max(8, elapsed)));
        velocity = elapsed > 100 || velocity * nextVelocity <= 0 ? nextVelocity : velocity * 0.4 + nextVelocity * 0.6;
      }
      last = y;
      lastTime = time;
      return movePixels(pixels);
    },
    end() {
      if (!active) return;
      active = false;
      if (!canScroll()) {
        cancel();
        return;
      }
      // Let go past an end, the rows go back to it from the speed they moved under the finger (slower than the finger,
      // as the band gives), and nothing coasts.
      if (pull !== 0) {
        const speed = now() - lastTime > 80 ? 0 : velocity * rubberBandSlope(pull, smooth?.height() ?? 0);
        stop();
        bounceBack(speed);
        return;
      }
      if (options.reducedMotion?.() === true || now() - lastTime > 80 || Math.abs(velocity) < 0.1) {
        stop();
        settle();
        return;
      }
      lastTime = now();
      frame = requestFrame(coast);
    },
    cancel,
  };
}

// Drives the scroller from single-finger touches on the container. A
// selection held inside the container wins over scrolling.
export function attachTouchScroll(
  container: HTMLElement,
  target: TouchScrollTarget,
  cellHeight: () => number,
  options: TouchScrollOptions & { selectionHeld: () => boolean },
): () => void {
  let finger: Finger = { clientX: 0, clientY: 0, touched: null };
  const scroll = newTouchScroll({ scrollLines: (amount) => target.scrollLines(amount, finger) }, cellHeight, {
    ...options,
    canScroll: () => !options.selectionHeld(),
  });
  const single = (event: TouchEvent): Touch | null =>
    event.touches.length === 1 ? (event.touches[0] ?? null) : null;
  const touchStart = (event: TouchEvent) => {
    const touch = single(event);
    if (touch === null) {
      scroll.cancel();
      return;
    }
    finger = { clientX: touch.clientX, clientY: touch.clientY, touched: event.target };
    scroll.start(touch.clientY, target.smoothScrollback?.(finger) ?? null);
  };
  const touchMove = (event: TouchEvent) => {
    const touch = single(event);
    if (touch === null) {
      scroll.cancel();
      return;
    }
    finger = { ...finger, clientX: touch.clientX, clientY: touch.clientY };
    scroll.move(touch.clientY);
  };
  const touchEnd = () => scroll.end();
  const selectionChanged = () => {
    if (options.selectionHeld()) scroll.cancel();
  };
  container.addEventListener("touchstart", touchStart, { passive: true });
  container.addEventListener("touchmove", touchMove, { passive: true });
  container.addEventListener("touchend", touchEnd, { passive: true });
  container.addEventListener("touchcancel", scroll.cancel, { passive: true });
  container.ownerDocument.addEventListener("selectionchange", selectionChanged);
  return () => {
    container.removeEventListener("touchstart", touchStart);
    container.removeEventListener("touchmove", touchMove);
    container.removeEventListener("touchend", touchEnd);
    container.removeEventListener("touchcancel", scroll.cancel);
    container.ownerDocument.removeEventListener("selectionchange", selectionChanged);
    scroll.cancel();
  };
}

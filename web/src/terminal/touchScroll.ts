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
  // cancel ends the gesture and puts the rows back on a row.
  const cancel = () => {
    stop();
    if (fraction !== 0) showFraction(0);
  };
  // moveSmoothly moves the scrollback by rows, whole rows through the scrollback and the rest by drawing the rows
  // moved. It never draws past the newest or the oldest row.
  const moveSmoothly = (scrollback: SmoothScrollback, rows: number) => {
    const room = scrollback.room();
    const total = fraction + rows;
    let lines = Math.floor(total);
    let next = total - lines;
    const atEnd = lines >= room.below || lines < -room.above;
    if (lines >= room.below) lines = room.below;
    else if (lines < -room.above) lines = -room.above;
    if (atEnd) next = 0;
    if (lines !== 0) view.scrollLines(lines);
    showFraction(next);
    // At either end there is nothing left to coast through.
    if (atEnd) velocity = 0;
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

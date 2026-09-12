
export type Scroller = { rows: number; scrollLines(amount: number): void };
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

  const cancel = () => {
    if (frame !== null) cancelFrame(frame);
    frame = null;
    active = false;
    velocity = 0;
    carried = 0;
  };
  const movePixels = (pixels: number) => {
    const cell = cellHeight();
    if (cell <= 0) return 0;
    carried += pixels / cell;
    const lines = Math.trunc(carried);
    if (lines !== 0) {
      carried -= lines;
      view.scrollLines(lines);
    }
    return lines;
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
    velocity *= Math.exp(-elapsed / 180);
    if (Math.abs(velocity) >= 0.02) frame = requestFrame(coast);
  };

  return {
    start(y: number) {
      cancel();
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
        const nextVelocity = Math.max(-3, Math.min(3, pixels / Math.max(8, elapsed)));
        velocity = elapsed > 100 || velocity * nextVelocity <= 0 ? nextVelocity : velocity * 0.4 + nextVelocity * 0.6;
      }
      last = y;
      lastTime = time;
      return movePixels(pixels);
    },
    end() {
      if (!active) return;
      active = false;
      if (!canScroll() || options.reducedMotion?.() === true || now() - lastTime > 80 || Math.abs(velocity) < 0.1) {
        cancel();
        return;
      }
      lastTime = now();
      frame = requestFrame(coast);
    },
    cancel,
  };
}

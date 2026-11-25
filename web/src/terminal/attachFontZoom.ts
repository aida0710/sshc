import { pinchedFontSize, steppedFontSize } from "./fontSize";

// wheelPixelsPerStep is how far the wheel turns, with Ctrl held, to change the text by one pixel. A trackpad's
// pinch comes as small turns of a wheel with Ctrl and needs about 25px for a pixel to feel like a pinch; one
// notch of a mouse wheel (100px, or 3 lines) changes it by one pixel, since one turn never takes more.
const wheelPixelsPerStep = 25;
// wheelPixelsPerLine reads a wheel that turns by lines (Firefox) in pixels, the height of a line of the page.
const wheelPixelsPerLine = 40;
// scrollTurnGapMs is the longest pause between two turns of the wheel that are one scroll. A trackpad keeps sending
// a scroll's turns every frame (about 16ms) after the fingers lift, while it coasts; a pinch or a turn made on
// purpose with Ctrl held starts after a longer pause than a few frames.
const scrollTurnGapMs = 80;

export type FontZoomTarget = {
  fontSize: () => number;
  setFontSize: (size: number) => void;
};

function fingerGap(touches: TouchList): number {
  const [first, second] = [touches[0], touches[1]];
  if (first === undefined || second === undefined) return 0;
  return Math.hypot(first.clientX - second.clientX, first.clientY - second.clientY);
}

// attachFontZoom changes the size of the text shown in container: two fingers pinch it, and the wheel turned
// with Ctrl held (a trackpad's pinch, too) steps it. Those gestures then neither scroll the terminal nor zoom
// the page. A scroll that was already coming when Ctrl went down is not such a turn. It returns what detaches them.
export function attachFontZoom(container: HTMLElement, target: FontZoomTarget): () => void {
  let turned = 0;
  // scrollLastTurnedAt is when the wheel last turned as part of a scroll: without Ctrl, or with Ctrl pressed while
  // that scroll was still coming. null when the last turn was not a scroll.
  let scrollLastTurnedAt: number | null = null;
  // The wheel is read before the terminal's own listener, which would scroll or report it to the program.
  const wheel = (event: WheelEvent) => {
    const now = performance.now();
    const scrollUnderWay = scrollLastTurnedAt !== null && now - scrollLastTurnedAt <= scrollTurnGapMs;
    if (!event.ctrlKey) {
      scrollLastTurnedAt = now;
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    // Ctrl went down while a scroll was still coming (Ctrl+Space to switch the input source, say): those turns
    // are the scroll's, so they change nothing.
    scrollLastTurnedAt = scrollUnderWay ? now : null;
    if (scrollUnderWay) return;
    turned += event.deltaMode === WheelEvent.DOM_DELTA_LINE ? event.deltaY * wheelPixelsPerLine : event.deltaY;
    if (Math.abs(turned) < wheelPixelsPerStep) return;
    // Turning the wheel away from you, as scrolling up, makes the text larger.
    const next = steppedFontSize(target.fontSize(), turned < 0 ? 1 : -1);
    turned = 0;
    if (next !== target.fontSize()) target.setFontSize(next);
  };

  let pinch: { gap: number; size: number } | null = null;
  const touchStart = (event: TouchEvent) => {
    const gap = event.touches.length === 2 ? fingerGap(event.touches) : 0;
    pinch = gap > 0 ? { gap, size: target.fontSize() } : null;
  };
  const touchMove = (event: TouchEvent) => {
    if (pinch === null || event.touches.length !== 2) return;
    const next = pinchedFontSize(pinch.size, fingerGap(event.touches) / pinch.gap);
    if (next !== target.fontSize()) target.setFontSize(next);
  };
  const touchEnd = (event: TouchEvent) => {
    if (event.touches.length < 2) pinch = null;
  };

  container.addEventListener("wheel", wheel, { capture: true, passive: false });
  container.addEventListener("touchstart", touchStart, { passive: true });
  container.addEventListener("touchmove", touchMove, { passive: true });
  container.addEventListener("touchend", touchEnd, { passive: true });
  container.addEventListener("touchcancel", touchEnd, { passive: true });
  return () => {
    container.removeEventListener("wheel", wheel, { capture: true });
    container.removeEventListener("touchstart", touchStart);
    container.removeEventListener("touchmove", touchMove);
    container.removeEventListener("touchend", touchEnd);
    container.removeEventListener("touchcancel", touchEnd);
  };
}

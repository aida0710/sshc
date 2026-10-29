import { pinchedFontSize, steppedFontSize } from "./fontSize";

// wheelPixelsPerStep is how far the wheel turns, with Ctrl held, to change the text by one pixel. A trackpad's
// pinch comes as small turns of a wheel with Ctrl and needs about 25px for a pixel to feel like a pinch; one
// notch of a mouse wheel (100px, or 3 lines) changes it by one pixel, since one turn never takes more.
const wheelPixelsPerStep = 25;
// wheelPixelsPerLine reads a wheel that turns by lines (Firefox) in pixels, the height of a line of the page.
const wheelPixelsPerLine = 40;

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
// the page. It returns what detaches them.
export function attachFontZoom(container: HTMLElement, target: FontZoomTarget): () => void {
  let turned = 0;
  // The wheel is read before the terminal's own listener, which would scroll or report it to the program.
  const wheel = (event: WheelEvent) => {
    if (!event.ctrlKey) return;
    event.preventDefault();
    event.stopPropagation();
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

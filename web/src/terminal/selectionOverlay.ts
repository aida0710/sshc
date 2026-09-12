import { viewportText, type ViewportBuffer } from "./buffer";
import { measureCells, type MeasurableTerminal } from "./metrics";


export const overlayClass = "sshc-select-overlay";

const tapSlopPixels = 8;
const tapHoldMillis = 400;

type OverlayTerminal = MeasurableTerminal & {
  readonly cols: number;
  readonly buffer: { readonly active: ViewportBuffer };
  focus(): void;
  blur(): void;
  onRender(handler: () => void): { dispose(): void };
};
export function selectionHeldIn(node: HTMLElement): boolean {
  const selection = node.ownerDocument.getSelection();
  if (selection === null || selection.isCollapsed) return false;
  return node.contains(selection.anchorNode);
}

export function attachSelectionOverlay(container: HTMLElement, view: OverlayTerminal): () => void {
  const overlay = container.ownerDocument.createElement("pre");
  overlay.className = `${overlayClass} absolute m-0 select-text overflow-hidden whitespace-pre text-transparent`;
  overlay.setAttribute("aria-hidden", "true");
  overlay.dir = "ltr";
  overlay.style.zIndex = "6";
  container.appendChild(overlay);

  let laidOut = "";

  const paint = () => {
    if (view.rows <= 0 || view.cols <= 0) return;

    const cells = measureCells(view);
    const base = container.getBoundingClientRect();

    const shape =
      cells === null
        ? laidOut
        : `${cells.rect.left - base.left} ${cells.rect.top - base.top} ${cells.rect.width} ${cells.rect.height}`;
    const relaidOut = shape !== laidOut;

    const held = selectionHeldIn(overlay);
    if (relaidOut && held) overlay.ownerDocument.getSelection()?.removeAllRanges();

    if (relaidOut || !held) {
      overlay.textContent = viewportText(view.buffer.active, view.rows, view.cols);
    }

    if (cells === null || !relaidOut) return;
    laidOut = shape;

    overlay.style.left = `${cells.rect.left - base.left}px`;
    overlay.style.top = `${cells.rect.top - base.top}px`;
    overlay.style.width = `${cells.rect.width}px`;
    overlay.style.height = `${cells.rect.height}px`;
    overlay.style.lineHeight = `${cells.cellHeight}px`;
    overlay.style.fontFamily = cells.font.family;
    overlay.style.fontSize = cells.font.size;
    overlay.style.fontWeight = cells.font.weight;
    overlay.style.letterSpacing = cells.font.letterSpacing;
    overlay.style.fontKerning = "none";
  };

  let touchedAt = 0;
  let touchedX = 0;
  let touchedY = 0;
  let dragged = false;
  let touching = false;
  let beganWithSelection = false;
  const began = (event: TouchEvent) => {
    const finger = event.touches[0];
    touching = event.touches.length === 1 && finger !== undefined;
    touchedAt = Date.now();
    touchedX = finger?.clientX ?? 0;
    touchedY = finger?.clientY ?? 0;
    dragged = false;
    beganWithSelection = selectionHeldIn(overlay);
  };
  const moved = (event: TouchEvent) => {
    const finger = event.touches[0];
    if (event.touches.length !== 1) touching = false;
    if (finger !== undefined && Math.hypot((finger.clientX ?? 0) - touchedX, finger.clientY - touchedY) > tapSlopPixels) dragged = true;
  };
  const ended = () => {
    const tapped = touching && !dragged && Date.now() - touchedAt < tapHoldMillis;
    touching = false;
    if (!tapped) return;
    // Dismissing text selection is an action of its own. A second deliberate
    // tap enters typing, rather than bringing up the IME while clearing handles.
    if (beganWithSelection || selectionHeldIn(overlay)) {
      overlay.ownerDocument.getSelection()?.removeAllRanges();
      paint();
      return;
    }
    // Android may keep the textarea focused after Back hides the IME. Refocus
    // only after a deliberate tap so that tapping can reopen that keyboard.
    view.blur();
    view.focus();
  };
  const cancelled = () => { touching = false; };
  const swallowCompatMouse = (event: MouseEvent) => event.preventDefault();
  container.addEventListener("touchstart", began, { passive: true });
  container.addEventListener("touchmove", moved, { passive: true });
  container.addEventListener("touchend", ended, { passive: true });
  container.addEventListener("touchcancel", cancelled, { passive: true });
  container.addEventListener("mousedown", swallowCompatMouse);

  const render = view.onRender(paint);
  paint();

  return () => {
    render.dispose();
    container.removeEventListener("touchstart", began);
    container.removeEventListener("touchmove", moved);
    container.removeEventListener("touchend", ended);
    container.removeEventListener("touchcancel", cancelled);
    container.removeEventListener("mousedown", swallowCompatMouse);
    overlay.remove();
  };
}

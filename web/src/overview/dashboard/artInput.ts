// What a person does on an art that answers to them: where the mouse is, and where they tapped or clicked. The art
// lies behind the tiles and does not take events itself, so the page's events are read and placed on its cells.

export type GridCell = { readonly column: number; readonly row: number };

export type ArtInput = {
  // pointer is the cell under the mouse, or null while it is elsewhere. A finger does not hover, so it leaves none.
  pointer: GridCell | null;
  // taps are the cells tapped or clicked since the last frame, oldest first. The art takes them each frame.
  taps: GridCell[];
};

// A press that moves farther than this (CSS pixels) is a scroll or a drag, not a tap.
const tapSlop = 8;
// A press on any of these does its own thing, so the art does not answer it.
const controls = "a, button, input, label, select, textarea, [role='button'], [role='link'], [role='radio'], [tabindex]";

type CellSize = { cellWidth: number; cellHeight: number; columns: number; rows: number };

// followArtInput keeps input up to date with what the page's events do over canvas, until the returned function
// is called.
export function followArtInput(canvas: HTMLCanvasElement, cells: CellSize, input: ArtInput): () => void {
  let pressed: { x: number; y: number } | null = null;

  const cellAt = (event: PointerEvent): GridCell | null => {
    const box = canvas.getBoundingClientRect();
    const x = event.clientX - box.left;
    const y = event.clientY - box.top;
    if (x < 0 || y < 0 || x >= box.width || y >= box.height) return null;
    return { column: Math.floor(x / cells.cellWidth), row: Math.floor(y / cells.cellHeight) };
  };
  const onControl = (event: PointerEvent) => event.target instanceof Element && event.target.closest(controls) !== null;

  const move = (event: PointerEvent) => {
    if (event.pointerType === "mouse") input.pointer = cellAt(event);
  };
  const leave = () => {
    input.pointer = null;
  };
  const down = (event: PointerEvent) => {
    pressed = onControl(event) ? null : { x: event.clientX, y: event.clientY };
  };
  const up = (event: PointerEvent) => {
    const start = pressed;
    pressed = null;
    if (start === null || Math.hypot(event.clientX - start.x, event.clientY - start.y) > tapSlop) return;
    const cell = cellAt(event);
    if (cell !== null) input.taps.push(cell);
  };

  window.addEventListener("pointermove", move, { passive: true });
  document.documentElement.addEventListener("pointerleave", leave);
  window.addEventListener("pointerdown", down, { passive: true });
  window.addEventListener("pointerup", up, { passive: true });
  return () => {
    window.removeEventListener("pointermove", move);
    document.documentElement.removeEventListener("pointerleave", leave);
    window.removeEventListener("pointerdown", down);
    window.removeEventListener("pointerup", up);
  };
}

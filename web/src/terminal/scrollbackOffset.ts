import type { Terminal } from "@xterm/xterm";
import { drawEdgeRow } from "./edgeRow";
import { measureCells, type MeasurableTerminal } from "./metrics";
import type { SmoothScrollback } from "./touchScroll";

type OffsetTerminal = MeasurableTerminal & Pick<Terminal, "cols" | "buffer" | "options">;

// Geometry is where the drawn rows sit in the host, measured once for a run of moves.
type Geometry = { left: number; bottom: number; cellHeight: number; cellWidth: number };

// createScrollbackOffset draws a terminal's scrollback moved by part of a row, for a finger moving it by the pixel.
// xterm.js draws its rows on whole rows only. So the host, which holds the drawn terminal (xterm.js's element, with
// its link underlines and command marks) and sshc's selection layer, is moved as a whole; the terminal's container
// clips it. The row past the bottom edge, which xterm.js does not draw, is drawn here (drawEdgeRow), so the rows
// moving up leave no gap.
export function createScrollbackOffset(host: HTMLElement, view: OffsetTerminal): SmoothScrollback & { dispose(): void } {
  const edge = host.ownerDocument.createElement("div");
  edge.setAttribute("aria-hidden", "true");
  edge.dataset.scrollbackEdgeRow = "";
  edge.style.cssText = "position:absolute;top:0;left:0;white-space:pre;pointer-events:none;display:none;";
  host.appendChild(edge);
  let geometry: Geometry | null = null;
  let drawnLine = "";

  const measure = (): Geometry | null => {
    const cells = measureCells(view);
    if (cells === null || view.cols <= 0) return null;
    const base = host.getBoundingClientRect();
    edge.style.fontFamily = cells.font.family;
    edge.style.fontSize = cells.font.size;
    edge.style.fontWeight = cells.font.weight;
    edge.style.lineHeight = `${cells.cellHeight}px`;
    edge.style.color = view.options.theme?.foreground ?? "";
    return {
      left: cells.rect.left - base.left,
      bottom: cells.rect.bottom - base.top,
      cellHeight: cells.cellHeight,
      cellWidth: cells.rect.width / view.cols,
    };
  };

  // clear puts the rows back on a row. The next run measures and draws afresh, in case the font or colours changed.
  const clear = () => {
    host.style.transform = "";
    edge.style.display = "none";
    geometry = null;
    drawnLine = "";
  };

  return {
    room() {
      const buffer = view.buffer.active;
      if (buffer.type !== "normal") return { above: 0, below: 0 };
      return { above: buffer.viewportY, below: buffer.baseY - buffer.viewportY };
    },
    show(fraction) {
      if (fraction <= 0) {
        clear();
        return;
      }
      // Measured before the host moves: both the rows and the host move together, so it holds for the whole run.
      geometry ??= measure();
      if (geometry === null) return;
      const buffer = view.buffer.active;
      const index = buffer.viewportY + view.rows;
      const line = buffer.getLine(index);
      const drawn = `${index}\n${line?.translateToString(false) ?? ""}`;
      if (drawn !== drawnLine) {
        drawEdgeRow(edge, line, { cols: view.cols, cellWidth: geometry.cellWidth, theme: view.options.theme ?? {} });
        drawnLine = drawn;
      }
      edge.style.left = `${geometry.left}px`;
      edge.style.top = `${geometry.bottom}px`;
      edge.style.height = `${geometry.cellHeight}px`;
      edge.style.display = "block";
      host.style.transform = `translateY(${-fraction * geometry.cellHeight}px)`;
    },
    dispose() {
      clear();
      edge.remove();
    },
  };
}

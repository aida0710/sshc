import type { IBuffer, IBufferRange, IDisposable, Terminal } from "@xterm/xterm";
import { findBufferLinks } from "./bufferLinks";
import { terminalScreen } from "./metrics";

// Lines in the accent colour under the URLs and paths a terminal opens (index.css). A touch terminal underlines
// every link on the screen, since a finger cannot point at one to find it; a mouse terminal underlines only the
// link under the pointer, in place of xterm's own line (linkProvider.ts). The lines lie over xterm's screen, so
// they look the same with the DOM and the WebGL renderer.

// UnderlineSegment is the part of a link on one row of the screen: the row from the top of the screen and the
// first column, both counted from 0, and the number of cells.
export type UnderlineSegment = { row: number; column: number; cells: number };

type ScreenShape = { top: number; rows: number; cols: number };

// rangeSegments splits a link's range, which counts columns and lines from 1 and may run over wrapped rows, into
// the rows of the screen it is on. top is the buffer line at the top of the screen, counted from 0.
export function rangeSegments(range: IBufferRange, { top, rows, cols }: ScreenShape): UnderlineSegment[] {
  const segments: UnderlineSegment[] = [];
  const firstLine = Math.max(range.start.y, top + 1);
  const lastLine = Math.min(range.end.y, top + rows);
  for (let line = firstLine; line <= lastLine; line += 1) {
    const from = line === range.start.y ? range.start.x : 1;
    const to = line === range.end.y ? range.end.x : cols;
    segments.push({ row: line - 1 - top, column: from - 1, cells: to - from + 1 });
  }
  return segments;
}

// linkSegmentsOnScreen is every link on the screen, as tappedLink.ts finds the one under a finger. A link wrapped
// over rows is found from each of them, and is underlined once.
export function linkSegmentsOnScreen(buffer: IBuffer, { rows, cols, allowWindowsPaths }: {
  rows: number;
  cols: number;
  allowWindowsPaths: boolean;
}): UnderlineSegment[] {
  const shape = { top: buffer.viewportY, rows, cols };
  const found = new Set<string>();
  const segments: UnderlineSegment[] = [];
  for (let row = 0; row < rows; row += 1) {
    for (const { range } of findBufferLinks(buffer, shape.top + row + 1, allowWindowsPaths)) {
      const key = `${range.start.x}:${range.start.y}:${range.end.x}:${range.end.y}`;
      if (found.has(key)) continue;
      found.add(key);
      segments.push(...rangeSegments(range, shape));
    }
  }
  return segments;
}

export type LinkUnderlines = {
  // point underlines the link under the mouse, or no link with null.
  point(range: IBufferRange | null): void;
  // refresh draws the lines again, after the settings they follow changed.
  refresh(): void;
  dispose(): void;
};

export function attachLinkUnderlines(view: Terminal, { everyLink, allowWindowsPaths }: {
  // everyLink is whether every link on the screen is underlined, not only the pointed one.
  everyLink: () => boolean;
  allowWindowsPaths: () => boolean;
}): LinkUnderlines {
  const screen = terminalScreen(view);
  const layer = document.createElement("div");
  layer.className = "sshc-link-underlines";
  screen?.appendChild(layer);
  const lines: HTMLElement[] = [];
  let pointed: IBufferRange | null = null;

  const lineAt = (index: number) => {
    let line = lines[index];
    if (line === undefined) {
      line = document.createElement("div");
      line.className = "sshc-link-underline";
      layer.appendChild(line);
      lines.push(line);
    }
    return line;
  };

  const segmentsToDraw = (): UnderlineSegment[] => {
    const buffer = view.buffer.active;
    if (everyLink()) return linkSegmentsOnScreen(buffer, { rows: view.rows, cols: view.cols, allowWindowsPaths: allowWindowsPaths() });
    return pointed === null ? [] : rangeSegments(pointed, { top: buffer.viewportY, rows: view.rows, cols: view.cols });
  };

  const draw = () => {
    let used = 0;
    if (screen !== null && view.rows > 0 && view.cols > 0) {
      const box = screen.getBoundingClientRect();
      const cellWidth = box.width / view.cols;
      const rowHeight = box.height / view.rows;
      for (const segment of segmentsToDraw()) {
        const line = lineAt(used);
        used += 1;
        line.style.left = `${segment.column * cellWidth}px`;
        line.style.top = `${segment.row * rowHeight}px`;
        line.style.width = `${segment.cells * cellWidth}px`;
        line.style.height = `${rowHeight}px`;
        line.hidden = false;
      }
    }
    for (let index = used; index < lines.length; index += 1) lines[index]!.hidden = true;
  };

  // Rendering, scrolling and resizing all move the links; draw once per frame.
  let frame: number | null = null;
  const schedule = () => {
    if (frame !== null) return;
    frame = requestAnimationFrame(() => {
      frame = null;
      draw();
    });
  };
  const subscriptions: IDisposable[] = [view.onRender(schedule), view.onScroll(schedule), view.onResize(schedule)];
  schedule();

  return {
    point(range) {
      pointed = range;
      schedule();
    },
    refresh: schedule,
    dispose() {
      if (frame !== null) cancelAnimationFrame(frame);
      for (const subscription of subscriptions) subscription.dispose();
      layer.remove();
    },
  };
}

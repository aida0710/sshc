import type { IBuffer, IBufferRange } from "@xterm/xterm";
import { findBufferLinks } from "./bufferLinks";
import type { TerminalLinkMatch } from "./links";
import { measureCells, type MeasurableTerminal } from "./metrics";
import { osc8LinkAtCell } from "./osc8LinkAtCell";

export type ScreenPoint = { clientX: number; clientY: number };

type TappableTerminal = MeasurableTerminal & {
  readonly cols: number;
  readonly buffer: { readonly active: IBuffer };
};

// BufferCell is a cell of the active buffer, counted from 0 as xterm's buffer
// API counts: x is the column, y the line including the scrollback.
export type BufferCell = { x: number; y: number };

// cellAtPoint is the cell of the active buffer under a point on the screen, or
// null outside the cells.
export function cellAtPoint(view: TappableTerminal, point: ScreenPoint): BufferCell | null {
  const cells = measureCells(view);
  if (cells === null || view.cols <= 0) return null;
  const x = Math.floor((point.clientX - cells.rect.left) / (cells.rect.width / view.cols));
  const row = Math.floor((point.clientY - cells.rect.top) / cells.cellHeight);
  if (x < 0 || x >= view.cols || row < 0 || row >= view.rows) return null;
  return { x, y: view.buffer.active.viewportY + row };
}

function covers(range: IBufferRange, column: number, line: number): boolean {
  const fromStart = line > range.start.y || (line === range.start.y && column >= range.start.x);
  const toEnd = line < range.end.y || (line === range.end.y && column <= range.end.x);
  return fromStart && toEnd;
}

// On touch screens the selection overlay lies over xterm's screen, so xterm's
// own link hover and click never see a finger. A tap looks up the link under it
// here instead: the OSC 8 hyperlink a program put on the cell, or else the URL
// or path read from the text. The hyperlink comes first because it names the
// address itself, however the program broke the text over rows.
export function linkAtPoint(view: TappableTerminal, point: ScreenPoint, allowWindowsPaths = false): TerminalLinkMatch | null {
  const cell = cellAtPoint(view, point);
  if (cell === null) return null;
  const hyperlink = osc8LinkAtCell(view, cell);
  if (hyperlink !== null) return hyperlink;
  // Link ranges count columns and lines from 1.
  const column = cell.x + 1;
  const line = cell.y + 1;
  return findBufferLinks(view.buffer.active, line, allowWindowsPaths).find(({ range }) => covers(range, column, line))?.match ?? null;
}

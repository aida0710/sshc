import type { IBuffer, IBufferRange } from "@xterm/xterm";
import { findBufferLinks } from "./bufferLinks";
import type { TerminalLinkMatch } from "./links";
import { measureCells, type MeasurableTerminal } from "./metrics";

export type ScreenPoint = { clientX: number; clientY: number };

type TappableTerminal = MeasurableTerminal & {
  readonly cols: number;
  readonly buffer: { readonly active: IBuffer };
};

function covers(range: IBufferRange, column: number, line: number): boolean {
  const fromStart = line > range.start.y || (line === range.start.y && column >= range.start.x);
  const toEnd = line < range.end.y || (line === range.end.y && column <= range.end.x);
  return fromStart && toEnd;
}

// On touch screens the selection overlay lies over xterm's screen, so xterm's
// own link hover and click never see a finger. A tap looks up the URL or path
// under it here instead. OSC 8 hyperlinks live inside xterm and are not found.
export function linkAtPoint(view: TappableTerminal, point: ScreenPoint): TerminalLinkMatch | null {
  const cells = measureCells(view);
  if (cells === null || view.cols <= 0) return null;
  const column = Math.floor((point.clientX - cells.rect.left) / (cells.rect.width / view.cols)) + 1;
  const row = Math.floor((point.clientY - cells.rect.top) / cells.cellHeight);
  if (column < 1 || column > view.cols || row < 0 || row >= view.rows) return null;
  const line = view.buffer.active.viewportY + row + 1;
  return findBufferLinks(view.buffer.active, line).find(({ range }) => covers(range, column, line))?.match ?? null;
}

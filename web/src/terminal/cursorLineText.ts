import type { Terminal } from "@xterm/xterm";

// textBeforeCursor is the line holding the cursor (with the rows it wrapped from) up to the cursor, or only its last
// maximumRows rows.
export function textBeforeCursor(view: Terminal, maximumRows = Number.POSITIVE_INFINITY): string {
  const buffer = view.buffer.active;
  const cursorRow = buffer.baseY + buffer.cursorY;
  const first = firstRowOfLine(view, cursorRow, cursorRow - maximumRows + 1);
  let text = "";
  for (let row = first; row < cursorRow; row += 1) text += buffer.getLine(row)?.translateToString(false) ?? "";
  return text + (buffer.getLine(cursorRow)?.translateToString(false, 0, buffer.cursorX) ?? "");
}

// cursorLineRows is how many rows the line holding the cursor takes, up to the cursor.
export function cursorLineRows(view: Terminal): number {
  const buffer = view.buffer.active;
  const cursorRow = buffer.baseY + buffer.cursorY;
  return cursorRow - firstRowOfLine(view, cursorRow) + 1;
}

// firstRowOfLine walks back no further than lowestRow.
function firstRowOfLine(view: Terminal, row: number, lowestRow = 0): number {
  const buffer = view.buffer.active;
  const lowest = Math.max(0, lowestRow);
  let first = row;
  while (first > lowest && buffer.getLine(first)?.isWrapped === true) first -= 1;
  return first;
}
